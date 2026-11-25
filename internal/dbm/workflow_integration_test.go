package dbm

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/queue"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

// fixedRegistry selalu mengembalikan satu target (schema terisolasi di test-db).
type fixedRegistry struct{ target *Target }

func (r fixedRegistry) Get(context.Context, int64) (*Target, error) { return r.target, nil }
func (fixedRegistry) Evict(int64)                                   {}
func (fixedRegistry) Close()                                        {}

// workflowFixture: satu database dengan dua group.
//
//	group A (edit): editor, leaderA (leader)
//	group B (view): viewer, leaderB (leader)
//
// Setiap pengajuan pada database ini butuh approval leaderA dan leaderB.
type workflowFixture struct {
	t        *testing.T
	ctx      context.Context
	pool     *pgxpool.Pool
	targetDB *sql.DB
	wf       *Workflow
	exec     *Executor
	worker   *queue.Worker
	connID   int64
	groupA   int64
	groupB   int64

	editor, leaderA, leaderB, viewer, outsider auth.User
}

func newWorkflowFixture(t *testing.T) *workflowFixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.NewPool(t)

	// Target = schema terpisah; search_path ikut tersalin dari konfigurasi pool.
	targetPool := testutil.NewPool(t)
	targetDB := stdlib.OpenDB(*targetPool.Config().ConnConfig.Copy())
	t.Cleanup(func() { targetDB.Close() })
	target := &Target{DB: targetDB, Dialect: Postgres{}}

	f := &workflowFixture{t: t, ctx: ctx, pool: pool, targetDB: targetDB}
	f.editor = f.user("editor")
	f.leaderA = f.user("leader-a")
	f.leaderB = f.user("leader-b")
	f.viewer = f.user("viewer")
	f.outsider = f.user("outsider")

	f.mustQueryRow(&f.connID, `INSERT INTO db_connections (name, connection_url) VALUES ('shop', 'postgres://unused') RETURNING id`)
	f.groupA = f.group("tim-a", "edit", f.leaderA, f.editor)
	f.groupB = f.group("tim-b", "view", f.leaderB, f.viewer)

	logger := activity.NewWithWriter(io.Discard)
	registry := fixedRegistry{target: target}
	f.wf = NewWorkflow(WorkflowConfig{
		Pool:     pool,
		Policy:   access.NewPolicy(access.NewPGLevelSource(pool)),
		Registry: registry,
		Queue:    queue.New(pool),
		Logger:   logger,
	})
	f.exec = NewExecutor(pool, registry, logger, &activity.Metrics{})
	run := f.exec.QueueHandler(queue.Busy, queue.Retry)
	f.worker = queue.NewWorker(pool, func(ctx context.Context, job queue.Job) error {
		return run(ctx, ExecJob{JobID: job.ID, RequestID: job.RequestID, Attempt: job.Attempts, MaxAttempts: job.MaxAttempts})
	}, logger)
	f.worker.Backoff = 0
	f.worker.OnGiveUp = func(ctx context.Context, job queue.Job, err error) {
		_ = f.exec.Abandon(ctx, job.RequestID, err)
	}
	return f
}

func (f *workflowFixture) mustExec(sqlText string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(f.ctx, sqlText, args...); err != nil {
		f.t.Fatalf("%s: %v", sqlText, err)
	}
}

func (f *workflowFixture) mustQueryRow(dst any, sqlText string, args ...any) {
	f.t.Helper()
	if err := f.pool.QueryRow(f.ctx, sqlText, args...).Scan(dst); err != nil {
		f.t.Fatalf("%s: %v", sqlText, err)
	}
}

func (f *workflowFixture) user(name string) auth.User {
	f.t.Helper()
	var id int64
	f.mustQueryRow(&id, `INSERT INTO users (username, password_hash) VALUES ($1, 'x') RETURNING id`, name)
	return auth.User{ID: id, Username: name, IsActive: true}
}

func (f *workflowFixture) group(name, level string, leader auth.User, members ...auth.User) int64 {
	f.t.Helper()
	var id int64
	f.mustQueryRow(&id, `INSERT INTO groups (name) VALUES ($1) RETURNING id`, name)
	for _, u := range append([]auth.User{leader}, members...) {
		f.mustExec(`INSERT INTO memberships (user_id, group_id) VALUES ($1, $2)`, u.ID, id)
	}
	f.mustExec(`UPDATE groups SET leader_id = $1 WHERE id = $2`, leader.ID, id)
	f.mustExec(`INSERT INTO grants (group_id, connection_id, level) VALUES ($1, $2, $3)`, id, f.connID, level)
	return id
}

// create membuat dan memvalidasi migration.
func (f *workflowFixture) create(name, up, down string) Migration {
	f.t.Helper()
	m, err := f.wf.CreateMigration(f.ctx, f.editor, f.connID, MigrationInput{Name: name, UpSQL: up, DownSQL: down})
	if err != nil {
		f.t.Fatalf("create %s: %v", name, err)
	}
	res, err := f.wf.Validate(f.ctx, f.editor, f.connID, m.ID)
	if err != nil || !res.OK {
		f.t.Fatalf("validate %s: ok=%v msg=%q err=%v", name, res.OK, res.Message, err)
	}
	return m
}

// approveAll mengajukan action lalu kedua leader menyetujui; mengembalikan job ID.
func (f *workflowFixture) approveAll(migID int64, action string) Request {
	f.t.Helper()
	req, err := f.wf.Submit(f.ctx, f.editor, f.connID, migID, action, "")
	if err != nil {
		f.t.Fatalf("submit %s: %v", action, err)
	}
	if d, err := f.wf.Approve(f.ctx, f.leaderA, req.ID, ""); err != nil || d.JobID != 0 {
		f.t.Fatalf("approve A: job=%d err=%v (harus menunggu group B)", d.JobID, err)
	}
	d, err := f.wf.Approve(f.ctx, f.leaderB, req.ID, "")
	if err != nil {
		f.t.Fatalf("approve B: %v", err)
	}
	if d.JobID == 0 || d.Request.Status != RequestQueued {
		f.t.Fatalf("setelah semua approve: job=%d status=%s, want job & queued", d.JobID, d.Request.Status)
	}
	return d.Request
}

// drain memproses job sampai antrian kosong.
func (f *workflowFixture) drain() {
	f.t.Helper()
	for i := 0; i < 10; i++ {
		processed, err := f.worker.ProcessOne(f.ctx)
		if err != nil {
			f.t.Fatalf("worker: %v", err)
		}
		if !processed {
			return
		}
	}
	f.t.Fatal("antrian tidak kunjung kosong")
}

func (f *workflowFixture) request(id int64) Request {
	f.t.Helper()
	req, err := f.wf.GetRequest(f.ctx, f.editor, id)
	if err != nil {
		f.t.Fatalf("get request %d: %v", id, err)
	}
	return req
}

func (f *workflowFixture) tableExists(name string) bool {
	f.t.Helper()
	var exists bool
	if err := f.targetDB.QueryRowContext(f.ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil {
		f.t.Fatal(err)
	}
	return exists
}

func TestWorkflowUpRollbackForward(t *testing.T) {
	f := newWorkflowFixture(t)
	m := f.create("widgets", "CREATE TABLE widgets (id INT)", "DROP TABLE widgets")

	up := f.approveAll(m.ID, ActionUp)
	if f.tableExists("widgets") {
		t.Fatal("SQL dijalankan sebelum worker memproses job (harus asynchronous)")
	}
	f.drain()
	if got := f.request(up.ID); got.Status != RequestSuccess || len(got.Executions) != 1 {
		t.Fatalf("up: status=%s executions=%d", got.Status, len(got.Executions))
	}
	if !f.tableExists("widgets") {
		t.Fatal("tabel widgets tidak terbentuk")
	}

	// Definisi immutable setelah dieksekusi, di level workflow maupun DB.
	if _, err := f.wf.UpdateMigration(f.ctx, f.editor, f.connID, m.ID,
		MigrationInput{UpSQL: "CREATE TABLE other (id INT)", DownSQL: "DROP TABLE other"}); !errors.Is(err, ErrMigrationLocked) {
		t.Errorf("update setelah eksekusi: err=%v, want ErrMigrationLocked", err)
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE migrations SET up_sql = 'SELECT 1' WHERE id = $1`, m.ID); err == nil {
		t.Error("trigger tidak menolak perubahan SQL migration yang terkunci")
	}

	// Forward tidak sah selama migration masih aktif.
	if _, err := f.wf.Submit(f.ctx, f.editor, f.connID, m.ID, ActionForward, ""); !errors.Is(err, ErrAlreadyActive) {
		t.Errorf("forward saat aktif: err=%v, want ErrAlreadyActive", err)
	}

	rb := f.approveAll(m.ID, ActionRollback)
	if rb.TargetExecutionID == nil || *rb.TargetExecutionID != f.request(up.ID).Executions[0].ID {
		t.Fatalf("rollback harus menargetkan eksekusi up: %v", rb.TargetExecutionID)
	}
	f.drain()
	if got := f.request(rb.ID); got.Status != RequestSuccess {
		t.Fatalf("rollback: status=%s err=%s", got.Status, got.Error)
	}
	if f.tableExists("widgets") {
		t.Fatal("rollback tidak menjalankan SQL down")
	}

	all, err := f.wf.History(f.ctx, f.editor, f.connID, false)
	if err != nil || len(all) != 2 {
		t.Fatalf("history: n=%d err=%v", len(all), err)
	}
	hidden, _ := f.wf.History(f.ctx, f.editor, f.connID, true)
	if len(hidden) != 0 {
		t.Errorf("hide rollback: %d baris tersisa, want pasangan up+rollback tersembunyi", len(hidden))
	}

	fw := f.approveAll(m.ID, ActionForward)
	f.drain()
	if got := f.request(fw.ID); got.Status != RequestSuccess || !f.tableExists("widgets") {
		t.Fatalf("forward: status=%s err=%s", got.Status, got.Error)
	}
	hidden, _ = f.wf.History(f.ctx, f.editor, f.connID, true)
	if len(hidden) != 1 || hidden[0].Action != ActionForward {
		t.Errorf("hide rollback setelah forward: %+v, want hanya forward", hidden)
	}

	// Baris eksekusi final append-only.
	if _, err := f.pool.Exec(f.ctx, `UPDATE migration_executions SET status = 'failed' WHERE request_id = $1`, up.ID); err == nil {
		t.Error("trigger tidak menolak perubahan eksekusi final")
	}
}

func TestWorkflowFailedExecutionIsRecorded(t *testing.T) {
	f := newWorkflowFixture(t)
	m := f.create("gadgets", "CREATE TABLE gadgets (id INT); CREATE TABLE gadget_tags (id INT)", "DROP TABLE gadget_tags; DROP TABLE gadgets")

	// Target berubah setelah validasi: statement kedua akan bentrok saat eksekusi.
	if _, err := f.targetDB.ExecContext(f.ctx, `CREATE TABLE gadget_tags (id INT)`); err != nil {
		t.Fatal(err)
	}
	req := f.approveAll(m.ID, ActionUp)
	f.drain()

	got := f.request(req.ID)
	if got.Status != RequestFailed || len(got.Executions) != 1 {
		t.Fatalf("status=%s executions=%d, want failed dengan 1 attempt", got.Status, len(got.Executions))
	}
	e := got.Executions[0]
	if e.Status != ExecFailed || e.SQLState != "42P07" || e.Error == "" {
		t.Errorf("eksekusi gagal tidak tercatat lengkap: status=%s sqlstate=%s error=%q", e.Status, e.SQLState, e.Error)
	}
	if f.tableExists("gadgets") {
		t.Error("statement pertama tetap ter-commit; eksekusi harus atomik")
	}
	mig, _ := f.wf.GetMigration(f.ctx, f.editor, f.connID, m.ID)
	if mig.State != StateFailed {
		t.Errorf("state = %s, want failed", mig.State)
	}
}

func TestWorkflowApprovalRules(t *testing.T) {
	f := newWorkflowFixture(t)
	m := f.create("orders", "CREATE TABLE orders (id INT)", "DROP TABLE orders")

	if _, err := f.wf.Submit(f.ctx, f.viewer, f.connID, m.ID, ActionUp, ""); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer submit: err=%v, want ErrForbidden", err)
	}
	if _, err := f.wf.Submit(f.ctx, f.editor, f.connID, m.ID, ActionRollback, ""); !errors.Is(err, ErrNotActive) {
		t.Errorf("rollback draft: err=%v, want ErrNotActive", err)
	}

	req, err := f.wf.Submit(f.ctx, f.editor, f.connID, m.ID, ActionUp, "tolong dicek")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.wf.Submit(f.ctx, f.editor, f.connID, m.ID, ActionUp, ""); !errors.Is(err, ErrRequestExists) {
		t.Errorf("pengajuan ganda: err=%v, want ErrRequestExists", err)
	}
	if _, err := f.wf.Approve(f.ctx, f.editor, req.ID, ""); !errors.Is(err, ErrNotReviewer) {
		t.Errorf("non-leader approve: err=%v, want ErrNotReviewer", err)
	}
	if _, err := f.wf.Approve(f.ctx, f.outsider, req.ID, ""); !errors.Is(err, ErrForbidden) && !errors.Is(err, ErrNotReviewer) {
		t.Errorf("user tanpa akses approve: err=%v", err)
	}

	inbox, err := f.wf.Inbox(f.ctx, f.leaderB)
	if err != nil || len(inbox) != 1 || inbox[0].ID != req.ID {
		t.Fatalf("inbox leader B: %+v err=%v", inbox, err)
	}

	if _, err := f.wf.Reject(f.ctx, f.leaderB, req.ID, " "); !errors.Is(err, ErrRejectNoteRequired) {
		t.Errorf("reject tanpa alasan: err=%v, want ErrRejectNoteRequired", err)
	}
	d, err := f.wf.Reject(f.ctx, f.leaderB, req.ID, "index belum ada")
	if err != nil || d.Request.Status != RequestRejected {
		t.Fatalf("reject: status=%s err=%v", d.Request.Status, err)
	}
	if _, err := f.wf.Approve(f.ctx, f.leaderA, req.ID, ""); !errors.Is(err, ErrRequestNotPending) {
		t.Errorf("approve setelah ditolak: err=%v, want ErrRequestNotPending", err)
	}

	// Setelah ditolak, definisi boleh direvisi dan revisi baru wajib divalidasi ulang.
	rev, err := f.wf.UpdateMigration(f.ctx, f.editor, f.connID, m.ID,
		MigrationInput{UpSQL: "CREATE TABLE orders (id INT PRIMARY KEY)", DownSQL: "DROP TABLE orders"})
	if err != nil || rev.Revision != 2 {
		t.Fatalf("revisi: rev=%d err=%v", rev.Revision, err)
	}
	if _, err := f.wf.Submit(f.ctx, f.editor, f.connID, m.ID, ActionUp, ""); !errors.Is(err, ErrValidationRequired) {
		t.Errorf("submit revisi belum divalidasi: err=%v, want ErrValidationRequired", err)
	}

	// Leader yang dikeluarkan dari group otomatis dilepas.
	f.mustExec(`DELETE FROM memberships WHERE group_id = $1 AND user_id = $2`, f.groupB, f.leaderB.ID)
	var leader *int64
	f.mustQueryRow(&leader, `SELECT leader_id FROM groups WHERE id = $1`, f.groupB)
	if leader != nil {
		t.Errorf("leader group B masih %d setelah dikeluarkan", *leader)
	}
}

func TestWorkflowRollbackOnlyLatest(t *testing.T) {
	f := newWorkflowFixture(t)
	first := f.create("first", "CREATE TABLE first_t (id INT)", "DROP TABLE first_t")
	f.approveAll(first.ID, ActionUp)
	f.drain()
	second := f.create("second", "CREATE TABLE second_t (id INT)", "DROP TABLE second_t")
	f.approveAll(second.ID, ActionUp)
	f.drain()

	if _, err := f.wf.Submit(f.ctx, f.editor, f.connID, first.ID, ActionRollback, ""); !errors.Is(err, ErrNotLatestActive) {
		t.Errorf("rollback bukan yang terakhir: err=%v, want ErrNotLatestActive", err)
	}
	irr, err := f.wf.CreateMigration(f.ctx, f.editor, f.connID, MigrationInput{
		UpSQL: "DROP TABLE second_t", Irreversible: true, IrreversibleReason: "data tabel hilang",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res, err := f.wf.Validate(f.ctx, f.editor, f.connID, irr.ID); err != nil || !res.OK {
		t.Fatalf("validate irreversible: %+v %v", res, err)
	}
	f.approveAll(irr.ID, ActionUp)
	f.drain()
	if _, err := f.wf.Submit(f.ctx, f.editor, f.connID, irr.ID, ActionRollback, ""); !errors.Is(err, ErrNotReversible) {
		t.Errorf("rollback irreversible: err=%v, want ErrNotReversible", err)
	}
}

func TestExecutorDuplicateJobRunsOnce(t *testing.T) {
	f := newWorkflowFixture(t)
	m := f.create("dupes", "CREATE TABLE dupes (id INT)", "DROP TABLE dupes")
	req := f.approveAll(m.ID, ActionUp)

	job := ExecJob{JobID: *f.request(req.ID).JobID, RequestID: req.ID, Attempt: 1, MaxAttempts: 3}
	for i := 0; i < 2; i++ {
		if err := f.exec.Run(f.ctx, job); err != nil {
			t.Fatalf("run #%d: %v", i+1, err)
		}
	}
	if got := f.request(req.ID); got.Status != RequestSuccess || len(got.Executions) != 1 {
		t.Fatalf("status=%s executions=%d, want satu eksekusi sukses", got.Status, len(got.Executions))
	}
}

func TestExecutorRevalidatesChangedDefinition(t *testing.T) {
	f := newWorkflowFixture(t)
	m := f.create("drift", "CREATE TABLE drift (id INT)", "DROP TABLE drift")
	req := f.approveAll(m.ID, ActionUp)

	// Definisi diubah langsung di DB setelah approval (belum terkunci).
	f.mustExec(`UPDATE migrations SET up_sql = 'DROP SCHEMA public CASCADE',
	            checksum = 'tampered', revision = revision + 1 WHERE id = $1`, m.ID)
	f.drain()

	got := f.request(req.ID)
	if got.Status != RequestFailed || len(got.Executions) != 1 || got.Executions[0].ErrorStage != "revalidation" {
		t.Fatalf("status=%s executions=%+v, want gagal di revalidasi", got.Status, got.Executions)
	}
	if f.tableExists("drift") {
		t.Error("SQL dijalankan walau definisi berubah setelah approval")
	}
}

// simulateCrash meniru worker yang mati setelah attempt dicatat running:
// execution running dengan target_txid, request running. committed=true berarti
// transaksi target sempat commit sebelum worker mati.
func (f *workflowFixture) simulateCrash(req Request, m Migration, committed bool) {
	f.t.Helper()
	tx, err := f.targetDB.BeginTx(f.ctx, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	var txid int64
	if err := tx.QueryRowContext(f.ctx, `SELECT txid_current()`).Scan(&txid); err != nil {
		f.t.Fatal(err)
	}
	if _, err := tx.ExecContext(f.ctx, m.UpSQL); err != nil {
		f.t.Fatal(err)
	}
	if committed {
		err = tx.Commit()
	} else {
		err = tx.Rollback()
	}
	if err != nil {
		f.t.Fatal(err)
	}
	f.mustExec(`INSERT INTO migration_executions (request_id, migration_id, connection_id, action,
	                    migration_version, migration_revision, sql_executed, status, attempt, target_txid, executed_by)
	            VALUES ($1, $2, $3, 'up', $4, $5, $6, 'running', 1, $7, $8)`,
		req.ID, m.ID, f.connID, m.Version, m.Revision, m.UpSQL, txid, f.editor.ID)
	f.mustExec(`UPDATE migration_requests SET status = 'running', started_at = now() WHERE id = $1`, req.ID)
	f.mustExec(`UPDATE migrations SET locked_at = now() WHERE id = $1`, m.ID)
}

func TestExecutorRecoversCommittedAttempt(t *testing.T) {
	f := newWorkflowFixture(t)
	m := f.create("recovered", "CREATE TABLE recovered (id INT)", "DROP TABLE recovered")
	req := f.approveAll(m.ID, ActionUp)
	f.simulateCrash(req, m, true)

	f.drain()
	got := f.request(req.ID)
	if got.Status != RequestSuccess || len(got.Executions) != 1 || got.Executions[0].Status != ExecSuccess {
		t.Fatalf("status=%s executions=%+v, want attempt lama dipulihkan menjadi sukses tanpa eksekusi ulang", got.Status, got.Executions)
	}
}

func TestExecutorRetriesAbortedAttempt(t *testing.T) {
	f := newWorkflowFixture(t)
	m := f.create("retried", "CREATE TABLE retried (id INT)", "DROP TABLE retried")
	req := f.approveAll(m.ID, ActionUp)
	f.simulateCrash(req, m, false)

	f.drain()
	got := f.request(req.ID)
	if got.Status != RequestSuccess || len(got.Executions) != 2 {
		t.Fatalf("status=%s executions=%d, want attempt gagal + attempt ulang sukses", got.Status, len(got.Executions))
	}
	if !f.tableExists("retried") {
		t.Error("attempt ulang tidak menjalankan SQL")
	}
}

func TestCreateManyMigrationsInSameMinute(t *testing.T) {
	f := newWorkflowFixture(t)
	var last int64
	for i := 0; i < 30; i++ {
		m, err := f.wf.CreateMigration(f.ctx, f.editor, f.connID, MigrationInput{UpSQL: "SELECT 1", DownSQL: "SELECT 2"})
		if err != nil {
			t.Fatalf("create #%d: %v", i+1, err)
		}
		if m.Version <= last {
			t.Fatalf("versi tidak naik: %d setelah %d", m.Version, last)
		}
		last = m.Version
	}
}
