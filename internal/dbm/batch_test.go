package dbm

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/testutil"
)

func TestBatchConfigNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   *BatchConfig
		want *BatchConfig
		err  error
	}{
		{"nil = migration biasa", nil, nil, nil},
		{"kosong = migration biasa", &BatchConfig{}, nil, nil},
		{"valid", &BatchConfig{Table: " items ", Column: " id ", Size: 500}, &BatchConfig{Table: "items", Column: "id", Size: 500}, nil},
		{"schema.tabel", &BatchConfig{Table: "public.items", Column: "id", Size: 1}, &BatchConfig{Table: "public.items", Column: "id", Size: 1}, nil},
		{"ukuran nol", &BatchConfig{Table: "items", Column: "id"}, nil, ErrBatchConfig},
		{"ukuran terlalu besar", &BatchConfig{Table: "items", Column: "id", Size: MaxBatchSize + 1}, nil, ErrBatchConfig},
		{"kolom kosong", &BatchConfig{Table: "items", Size: 10}, nil, ErrBatchConfig},
		{"injeksi nama tabel", &BatchConfig{Table: "items; DROP TABLE x", Column: "id", Size: 10}, nil, ErrBatchIdentifier},
		{"injeksi kolom", &BatchConfig{Table: "items", Column: "id)--", Size: 10}, nil, ErrBatchIdentifier},
		{"terlalu banyak titik", &BatchConfig{Table: "a.b.c", Column: "id", Size: 10}, nil, ErrBatchIdentifier},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.in.Normalize()
			if !errors.Is(err, tc.err) {
				t.Fatalf("err = %v, want %v", err, tc.err)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestHasBatchPlaceholders(t *testing.T) {
	cases := map[string]bool{
		"UPDATE t SET x = 1 WHERE id BETWEEN $1 AND $2":   true,
		"UPDATE t SET x = 1 WHERE id >= $1 AND id <= $2;": true,
		"UPDATE t SET x = 1 WHERE id >= $1":               false,
		"UPDATE t SET x = 1 WHERE id >= $10 AND id <= $2": false,
		"UPDATE t SET x = 1":                              false,
	}
	for sqlText, want := range cases {
		if got := hasBatchPlaceholders(sqlText); got != want {
			t.Errorf("%q: got %v, want %v", sqlText, got, want)
		}
	}
}

func TestMigrationInputBatchRules(t *testing.T) {
	batch := &BatchConfig{Table: "items", Column: "id", Size: 100}
	ok := MigrationInput{UpSQL: "UPDATE items SET v = v + 1 WHERE id BETWEEN $1 AND $2",
		DownSQL: "UPDATE items SET v = v - 1 WHERE id BETWEEN $1 AND $2", Batch: batch}
	if _, err := ok.Normalize(); err != nil {
		t.Fatalf("input batch valid ditolak: %v", err)
	}
	noUpPlaceholder := ok
	noUpPlaceholder.UpSQL = "UPDATE items SET v = v + 1"
	if _, err := noUpPlaceholder.Normalize(); !errors.Is(err, ErrBatchPlaceholders) {
		t.Fatalf("SQL up tanpa placeholder: err = %v", err)
	}
	noDownPlaceholder := ok
	noDownPlaceholder.DownSQL = "UPDATE items SET v = v - 1"
	if _, err := noDownPlaceholder.Normalize(); !errors.Is(err, ErrBatchPlaceholders) {
		t.Fatalf("SQL down tanpa placeholder: err = %v", err)
	}
	irreversible := MigrationInput{UpSQL: ok.UpSQL, Irreversible: true, IrreversibleReason: "data lama hilang", Batch: batch}
	if _, err := irreversible.Normalize(); err != nil {
		t.Fatalf("batch irreversible ditolak: %v", err)
	}
}

func TestInputChecksumIncludesBatch(t *testing.T) {
	base := MigrationInput{UpSQL: "UPDATE t SET x = 1 WHERE id BETWEEN $1 AND $2", DownSQL: "UPDATE t SET x = 0 WHERE id BETWEEN $1 AND $2"}
	plain := InputChecksum(base)
	if plain != DefinitionChecksum(base.UpSQL, base.DownSQL, "") {
		t.Fatal("checksum migration biasa harus sama dengan formula lama")
	}
	withBatch := base
	withBatch.Batch = &BatchConfig{Table: "t", Column: "id", Size: 10}
	other := base
	other.Batch = &BatchConfig{Table: "t", Column: "id", Size: 20}
	if InputChecksum(withBatch) == plain || InputChecksum(withBatch) == InputChecksum(other) {
		t.Fatal("konfigurasi batch harus ikut checksum (ukuran batch berbeda = revisi berbeda)")
	}
}

func TestPlanActionPartial(t *testing.T) {
	// Migration 1 aktif, migration 2 berhenti sebagian (eksekusi 20).
	partialUp := []MigrationState{
		{MigrationID: 1, Reversible: true, HasAttempts: true, LastSuccessID: 10, LastSuccessAction: ActionUp},
		{MigrationID: 2, Reversible: true, HasAttempts: true, PartialID: 20, PartialAction: ActionUp},
		{MigrationID: 3, Reversible: true},
	}
	if s := partialUp[1].State(); s != StatePartial {
		t.Fatalf("state = %s, want partial", s)
	}
	if p, err := PlanAction(partialUp, 2, ActionResume); err != nil || p.Action != ActionResume || p.TargetExecution != 20 {
		t.Fatalf("resume: plan=%+v err=%v", p, err)
	}
	if p, err := PlanAction(partialUp, 2, ActionRollback); err != nil || p.TargetExecution != 20 {
		t.Fatalf("rollback batch partial: plan=%+v err=%v", p, err)
	}
	if _, err := PlanAction(partialUp, 2, ActionUp); !errors.Is(err, ErrPartialPending) {
		t.Fatalf("up pada migration partial: err = %v", err)
	}
	// Migration lain ditahan selama ada yang partial.
	if _, err := PlanAction(partialUp, 3, ActionUp); !errors.Is(err, ErrDatabasePartial) {
		t.Fatalf("migration lain harus ditahan: err = %v", err)
	}
	if _, err := PlanAction(partialUp, 1, ActionRollback); !errors.Is(err, ErrDatabasePartial) {
		t.Fatalf("rollback migration lain harus ditahan: err = %v", err)
	}
	// Resume hanya untuk partial.
	if _, err := PlanAction(partialUp[:1], 1, ActionResume); !errors.Is(err, ErrNotPartial) {
		t.Fatalf("resume migration aktif: err = %v", err)
	}

	// Rollback per batch yang berhenti sebagian (partially reverted) hanya bisa dilanjutkan.
	reverting := []MigrationState{{MigrationID: 2, Reversible: true, HasAttempts: true,
		LastSuccessID: 20, LastSuccessAction: ActionUp, PartialID: 30, PartialAction: ActionRollback, PartialReverts: 20}}
	if p, err := PlanAction(reverting, 2, ActionRollback); err != nil || p.TargetExecution != 20 {
		t.Fatalf("lanjut rollback: plan=%+v err=%v", p, err)
	}
	if _, err := PlanAction(reverting, 2, ActionResume); !errors.Is(err, ErrPartialPending) {
		t.Fatalf("resume saat partially reverted: err = %v", err)
	}

	irreversible := []MigrationState{{MigrationID: 2, HasAttempts: true, PartialID: 20, PartialAction: ActionUp}}
	if _, err := PlanAction(irreversible, 2, ActionRollback); !errors.Is(err, ErrNotReversible) {
		t.Fatalf("pembatalan migration irreversible: err = %v", err)
	}
}

func TestSetOutcomeIgnoresOlderPartial(t *testing.T) {
	s := MigrationState{MigrationID: 1, Reversible: true, HasAttempts: true, LastSuccessID: 40, LastSuccessAction: ActionResume}
	s.setOutcome(40, ActionResume, ExecSuccess, 0)
	if s.State() != StateActive {
		t.Fatalf("resume sukses harus membuat migration aktif, got %s", s.State())
	}
	s.setOutcome(30, ActionUp, ExecPartial, 0)
	if s.State() != StateActive {
		t.Fatal("partial yang lebih lama dari sukses terakhir tidak boleh mengubah posisi")
	}
}

// batchTarget menyiapkan tabel items(id 1..n, v 0, flag 1) di schema terisolasi.
func batchTarget(t *testing.T, n int) *sql.DB {
	t.Helper()
	pool := testutil.NewPool(t)
	db := stdlib.OpenDB(*pool.Config().ConnConfig.Copy())
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	for _, q := range []string{
		`CREATE TABLE items (id BIGINT PRIMARY KEY, v INT NOT NULL DEFAULT 0, flag INT NOT NULL DEFAULT 1)`,
		`INSERT INTO items (id) SELECT generate_series(1, ` + strconv.Itoa(n) + `)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func sumV(t *testing.T, db *sql.DB) (sum, touched int64) {
	t.Helper()
	if err := db.QueryRow(`SELECT COALESCE(SUM(v), 0), COUNT(*) FILTER (WHERE v <> 0) FROM items`).Scan(&sum, &touched); err != nil {
		t.Fatal(err)
	}
	return sum, touched
}

const (
	batchUpSQL   = `UPDATE items SET v = v + 1 WHERE id BETWEEN $1 AND $2 AND 1 / flag = 1`
	batchDownSQL = `UPDATE items SET v = v - 1 WHERE id BETWEEN $1 AND $2`
)

func TestBatchRunUpThenDown(t *testing.T) {
	db := batchTarget(t, 1050)
	ctx := context.Background()
	var progress []int
	run := BatchRun{DB: db, Dialect: Postgres{}, Config: BatchConfig{Table: "items", Column: "id", Size: 100},
		Key: 7, Direction: BatchUp, SQL: batchUpSQL, LockSQL: Postgres{}.LockSQL(1),
		Progress: func(cp BatchCheckpoint, total, rows int64) error { progress = append(progress, cp.Number); return nil }}

	res, err := run.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Batches != 11 || res.Rows != 1050 || res.TotalBatch != 11 || res.LastCursor != "1050" {
		t.Fatalf("hasil up = %+v", res)
	}
	if len(progress) != 11 || progress[0] != 1 || progress[10] != 11 {
		t.Fatalf("progres per batch = %v", progress)
	}
	if sum, touched := sumV(t, db); sum != 1050 || touched != 1050 {
		t.Fatalf("setiap row harus diubah tepat sekali: sum=%d touched=%d", sum, touched)
	}

	// Dipanggil ulang: semua batch sudah punya checkpoint, tidak ada yang diulang.
	again, err := run.Run(ctx)
	if err != nil || again.RunBatches != 0 || again.Batches != 11 {
		t.Fatalf("run ulang harus idempoten: %+v err=%v", again, err)
	}
	if sum, _ := sumV(t, db); sum != 1050 {
		t.Fatalf("run ulang mengulang batch: sum=%d", sum)
	}

	// Pembatalan per batch dalam urutan terbalik.
	var order []int
	down := run
	down.Direction, down.SQL = BatchDown, batchDownSQL
	down.Progress = func(cp BatchCheckpoint, total, rows int64) error { order = append(order, cp.Number); return nil }
	dres, err := down.Run(ctx)
	if err != nil || dres.Batches != 11 {
		t.Fatalf("down: %+v err=%v", dres, err)
	}
	if order[0] != 11 || order[len(order)-1] != 1 {
		t.Fatalf("urutan pembatalan harus terbalik, got %v", order)
	}
	if sum, touched := sumV(t, db); sum != 0 || touched != 0 {
		t.Fatalf("setelah down: sum=%d touched=%d", sum, touched)
	}
}

func TestBatchRunPartialThenResume(t *testing.T) {
	db := batchTarget(t, 500)
	ctx := context.Background()
	// Row 250 membuat SQL gagal (pembagian dengan nol) di batch ke-3.
	if _, err := db.Exec(`UPDATE items SET flag = 0 WHERE id = 250`); err != nil {
		t.Fatal(err)
	}
	run := BatchRun{DB: db, Dialect: Postgres{}, Config: BatchConfig{Table: "items", Column: "id", Size: 100},
		Key: 9, Direction: BatchUp, SQL: batchUpSQL}
	res, err := run.Run(ctx)
	if err == nil || sqlState(err) != "22012" {
		t.Fatalf("batch ke-3 harus gagal division_by_zero, err=%v", err)
	}
	if res.Batches != 2 || res.Rows != 200 || !res.Committed() {
		t.Fatalf("batch sebelum kegagalan harus tetap committed: %+v", res)
	}
	if sum, _ := sumV(t, db); sum != 200 {
		t.Fatalf("batch gagal harus di-rollback sendiri: sum=%d", sum)
	}

	// Cakupan dikunci sejak awal: row baru tidak ikut diproses saat resume.
	if _, err := db.Exec(`INSERT INTO items (id) VALUES (900); UPDATE items SET flag = 1 WHERE id = 250`); err != nil {
		t.Fatal(err)
	}
	resumed, err := run.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Batches != 5 || resumed.RunBatches != 3 || resumed.Rows != 500 {
		t.Fatalf("resume harus melanjutkan dari batch 3: %+v", resumed)
	}
	var v900 int
	if err := db.QueryRow(`SELECT v FROM items WHERE id = 900`).Scan(&v900); err != nil || v900 != 0 {
		t.Fatalf("row di luar cakupan awal ikut diproses: v=%d err=%v", v900, err)
	}
	if sum, touched := sumV(t, db); sum != 500 || touched != 500 {
		t.Fatalf("setiap row dalam cakupan diproses tepat sekali: sum=%d touched=%d", sum, touched)
	}
}

func TestBatchRunEmptyTable(t *testing.T) {
	db := batchTarget(t, 0)
	run := BatchRun{DB: db, Dialect: Postgres{}, Config: BatchConfig{Table: "items", Column: "id", Size: 10},
		Key: 1, Direction: BatchUp, SQL: batchUpSQL}
	res, err := run.Run(context.Background())
	if err != nil || res.Batches != 0 {
		t.Fatalf("tabel kosong: %+v err=%v", res, err)
	}
}

// createBatch membuat migration per batch lalu memvalidasinya.
func (f *workflowFixture) createBatch(name, up, down string, size int) Migration {
	f.t.Helper()
	m, err := f.wf.CreateMigration(f.ctx, f.editor, f.connID, MigrationInput{Name: name, UpSQL: up, DownSQL: down,
		Batch: &BatchConfig{Table: "items", Column: "id", Size: size}})
	if err != nil {
		f.t.Fatalf("create %s: %v", name, err)
	}
	res, err := f.wf.Validate(f.ctx, f.editor, f.connID, m.ID)
	if err != nil || !res.OK || !res.DryRun {
		f.t.Fatalf("validate %s: ok=%v dry=%v msg=%q err=%v", name, res.OK, res.DryRun, res.Message, err)
	}
	return m
}

func (f *workflowFixture) seedItems(n int) {
	f.t.Helper()
	for _, q := range []string{
		`CREATE TABLE items (id BIGINT PRIMARY KEY, v INT NOT NULL DEFAULT 0, flag INT NOT NULL DEFAULT 1)`,
		`INSERT INTO items (id) SELECT generate_series(1, ` + strconv.Itoa(n) + `)`,
	} {
		if _, err := f.targetDB.ExecContext(f.ctx, q); err != nil {
			f.t.Fatal(err)
		}
	}
}

func (f *workflowFixture) lastExecution(req Request) Execution {
	f.t.Helper()
	got := f.request(req.ID)
	if len(got.Executions) == 0 {
		f.t.Fatalf("request %d belum punya eksekusi", req.ID)
	}
	e, err := f.wf.GetExecution(f.ctx, f.editor, f.connID, got.Executions[len(got.Executions)-1].ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

func TestWorkflowBatchMigrationLifecycle(t *testing.T) {
	f := newWorkflowFixture(t)
	f.seedItems(450)
	m := f.createBatch("tambah v", batchUpSQL, batchDownSQL, 100)
	if m.Batch == nil || m.Batch.Size != 100 {
		t.Fatalf("konfigurasi batch tidak tersimpan: %+v", m.Batch)
	}

	up := f.approveAll(m.ID, ActionUp)
	f.drain()
	if got := f.request(up.ID); got.Status != RequestSuccess {
		t.Fatalf("up batch: status=%s err=%s", got.Status, got.Error)
	}
	e := f.lastExecution(up)
	if e.CommittedBatches != 5 || e.CommittedRows != 450 || e.BatchKey == nil || len(e.Checkpoints) != 5 {
		t.Fatalf("progres batch: batches=%d rows=%d key=%v checkpoints=%d", e.CommittedBatches, e.CommittedRows, e.BatchKey, len(e.Checkpoints))
	}
	if sum, _ := sumV(t, f.targetDB); sum != 450 {
		t.Fatalf("sum v = %d, want 450", sum)
	}

	// Definisi terkunci setelah eksekusi, termasuk konfigurasi batch.
	if _, err := f.wf.UpdateMigration(f.ctx, f.editor, f.connID, m.ID, MigrationInput{UpSQL: batchUpSQL, DownSQL: batchDownSQL,
		Batch: &BatchConfig{Table: "items", Column: "id", Size: 5}}); !errors.Is(err, ErrMigrationLocked) {
		t.Fatalf("ubah batch setelah eksekusi: err = %v", err)
	}

	// Rollback migration batch = pembatalan per batch dalam urutan terbalik.
	rb := f.approveAll(m.ID, ActionRollback)
	f.drain()
	if got := f.request(rb.ID); got.Status != RequestSuccess {
		t.Fatalf("rollback batch: status=%s err=%s", got.Status, got.Error)
	}
	re := f.lastExecution(rb)
	if re.RevertsExecutionID == nil || *re.RevertsExecutionID != e.ID || re.CommittedBatches != 5 {
		t.Fatalf("rollback harus merujuk eksekusi up dan membatalkan 5 batch: %+v", re)
	}
	if re.Checkpoints[0].Number != 5 {
		t.Fatalf("batch pertama yang dibatalkan harus batch terakhir, got %d", re.Checkpoints[0].Number)
	}
	if sum, _ := sumV(t, f.targetDB); sum != 0 {
		t.Fatalf("setelah rollback sum v = %d", sum)
	}

	// Forward memulai rangkaian checkpoint baru.
	fw := f.approveAll(m.ID, ActionForward)
	f.drain()
	if got := f.request(fw.ID); got.Status != RequestSuccess {
		t.Fatalf("forward batch: status=%s err=%s", got.Status, got.Error)
	}
	if sum, _ := sumV(t, f.targetDB); sum != 450 {
		t.Fatalf("setelah forward sum v = %d", sum)
	}
}

func TestWorkflowBatchPartialHoldsDatabaseThenResume(t *testing.T) {
	f := newWorkflowFixture(t)
	f.seedItems(500)
	if _, err := f.targetDB.Exec(`UPDATE items SET flag = 0 WHERE id = 250`); err != nil {
		t.Fatal(err)
	}
	m := f.createBatch("tambah v", batchUpSQL, batchDownSQL, 100)
	other := f.create("lain", "CREATE TABLE lain (id INT)", "DROP TABLE lain")

	up := f.approveAll(m.ID, ActionUp)
	f.drain()
	got := f.request(up.ID)
	if got.Status != RequestPartial {
		t.Fatalf("status = %s, want partial (err=%s)", got.Status, got.Error)
	}
	e := f.lastExecution(up)
	if e.Status != ExecPartial || e.CommittedBatches != 2 || e.CommittedRows != 200 || e.SQLState != "22012" {
		t.Fatalf("eksekusi partial: status=%s batches=%d rows=%d sqlstate=%s", e.Status, e.CommittedBatches, e.CommittedRows, e.SQLState)
	}
	mm, _ := f.wf.GetMigration(f.ctx, f.editor, f.connID, m.ID)
	if mm.State != StatePartial || mm.PartialExecutionID != e.ID {
		t.Fatalf("state migration = %s partial=%d", mm.State, mm.PartialExecutionID)
	}

	// Database ditahan: migration lain tidak dapat diajukan.
	if _, err := f.wf.Submit(f.ctx, f.editor, f.connID, other.ID, ActionUp, ""); !errors.Is(err, ErrDatabasePartial) {
		t.Fatalf("migration lain saat partial: err = %v", err)
	}

	// Perbaiki data lalu resume (tetap lewat approval).
	if _, err := f.targetDB.Exec(`UPDATE items SET flag = 1 WHERE id = 250`); err != nil {
		t.Fatal(err)
	}
	res := f.approveAll(m.ID, ActionResume)
	if res.TargetExecutionID == nil || *res.TargetExecutionID != e.ID {
		t.Fatalf("resume harus menargetkan eksekusi partial: %+v", res.TargetExecutionID)
	}
	f.drain()
	if got := f.request(res.ID); got.Status != RequestSuccess {
		t.Fatalf("resume: status=%s err=%s", got.Status, got.Error)
	}
	re := f.lastExecution(res)
	if re.Action != ActionResume || re.CommittedBatches != 5 || len(re.Checkpoints) != 3 {
		t.Fatalf("resume harus melanjutkan batch 3-5: action=%s batches=%d checkpoints=%d", re.Action, re.CommittedBatches, len(re.Checkpoints))
	}
	if sum, touched := sumV(t, f.targetDB); sum != 500 || touched != 500 {
		t.Fatalf("tidak boleh ada batch yang diulang: sum=%d touched=%d", sum, touched)
	}
	if mm, _ := f.wf.GetMigration(f.ctx, f.editor, f.connID, m.ID); mm.State != StateActive {
		t.Fatalf("setelah resume state = %s, want active", mm.State)
	}
	// Database tidak lagi ditahan.
	f.approveAll(other.ID, ActionUp)
	f.drain()
	if !f.tableExists("lain") {
		t.Fatal("migration lain harus bisa berjalan setelah partial selesai")
	}
}

func TestWorkflowBatchPartialCancelReversesCommittedBatches(t *testing.T) {
	f := newWorkflowFixture(t)
	f.seedItems(300)
	if _, err := f.targetDB.Exec(`UPDATE items SET flag = 0 WHERE id = 250`); err != nil {
		t.Fatal(err)
	}
	m := f.createBatch("tambah v", batchUpSQL, batchDownSQL, 100)
	up := f.approveAll(m.ID, ActionUp)
	f.drain()
	if got := f.request(up.ID); got.Status != RequestPartial {
		t.Fatalf("status = %s, want partial", got.Status)
	}

	rb := f.approveAll(m.ID, ActionRollback)
	f.drain()
	if got := f.request(rb.ID); got.Status != RequestSuccess {
		t.Fatalf("pembatalan batch: status=%s err=%s", got.Status, got.Error)
	}
	if sum, _ := sumV(t, f.targetDB); sum != 0 {
		t.Fatalf("batch committed harus dibatalkan semua: sum=%d", sum)
	}
	mm, _ := f.wf.GetMigration(f.ctx, f.editor, f.connID, m.ID)
	if mm.State != StateRolledBack {
		t.Fatalf("state = %s, want rolled_back", mm.State)
	}
	// Riwayat tetap utuh: partial dan pembatalannya tercatat; Hide rollback menyembunyikan pasangannya.
	all, _ := f.wf.History(f.ctx, f.editor, f.connID, false)
	hidden, _ := f.wf.History(f.ctx, f.editor, f.connID, true)
	if len(all) != 2 || len(hidden) != 0 {
		t.Fatalf("riwayat: semua=%d tersembunyi=%d, want 2 dan 0", len(all), len(hidden))
	}
}
