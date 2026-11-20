package dbm

// Definisi migration bisa direvisi sampai eksekusi pertama, lalu terkunci
// (dijaga juga oleh trigger database).

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrMigrationNotFound  = errors.New("migration tidak ditemukan")
	ErrMigrationLocked    = errors.New("migration sudah pernah dieksekusi; SQL up/down immutable, buat migration baru untuk perbaikan")
	ErrMigrationInReview  = errors.New("migration sedang diajukan; batalkan pengajuan sebelum mengubah SQL")
	ErrChecksumMismatch   = errors.New("checksum SQL tidak cocok dengan revisi yang diajukan")
	ErrVersionConflict    = errors.New("versi migration bentrok")
	ErrEmptySQL           = errors.New("SQL up migration kosong")
	ErrMissingDown        = errors.New("SQL down wajib diisi, atau tandai migration sebagai irreversible beserta alasannya")
	ErrMissingReason      = errors.New("alasan irreversible wajib diisi")
	ErrIrreversibleDown   = errors.New("migration irreversible tidak boleh memiliki SQL down")
	ErrDownSameAsUp       = errors.New("SQL down tidak boleh sama dengan SQL up")
	ErrNameTooLong        = errors.New("nama migration maksimal 80 karakter")
	ErrValidationRequired = errors.New("migration harus lolos validasi untuk revisi terbaru sebelum diajukan")
)

const maxNameLength = 80

const maxErrorLength = 2000

// MigrationInput = isian form/API saat membuat atau mengubah draft.
type MigrationInput struct {
	Name               string `json:"name"`
	UpSQL              string `json:"up_sql"`
	DownSQL            string `json:"down_sql"`
	Irreversible       bool   `json:"irreversible"`
	IrreversibleReason string `json:"irreversible_reason"`
}

// Normalize merapikan spasi lalu memeriksa aturan statis definisi.
func (in MigrationInput) Normalize() (MigrationInput, error) {
	in.Name = strings.Join(strings.Fields(in.Name), " ")
	in.UpSQL = strings.TrimSpace(in.UpSQL)
	in.DownSQL = strings.TrimSpace(in.DownSQL)
	in.IrreversibleReason = strings.TrimSpace(in.IrreversibleReason)

	if in.UpSQL == "" {
		return in, ErrEmptySQL
	}
	if len([]rune(in.Name)) > maxNameLength {
		return in, ErrNameTooLong
	}
	if in.Name == "" {
		in.Name = DeriveName(in.UpSQL)
	}
	if in.Irreversible {
		if in.IrreversibleReason == "" {
			return in, ErrMissingReason
		}
		if in.DownSQL != "" {
			return in, ErrIrreversibleDown
		}
		return in, nil
	}
	in.IrreversibleReason = ""
	if in.DownSQL == "" {
		return in, ErrMissingDown
	}
	if in.DownSQL == in.UpSQL {
		return in, ErrDownSameAsUp
	}
	return in, nil
}

// Migration = definisi migration beserta posisi turunannya.
type Migration struct {
	ID                 int64      `json:"id"`
	ConnectionID       int64      `json:"connection_id"`
	Version            int64      `json:"version"`
	Name               string     `json:"name"`
	UpSQL              string     `json:"up_sql,omitempty"`
	DownSQL            string     `json:"down_sql,omitempty"`
	IrreversibleReason string     `json:"irreversible_reason,omitempty"`
	Revision           int        `json:"revision"`
	Checksum           string     `json:"checksum"`
	LockedAt           *time.Time `json:"locked_at,omitempty"`
	Validation         Validation `json:"validation"`
	CreatedBy          *int64     `json:"created_by,omitempty"`
	CreatedByName      string     `json:"created_by_name,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// Diisi dari riwayat eksekusi dan pengajuan.
	State             string      `json:"state"`
	ActiveExecutionID int64       `json:"active_execution_id,omitempty"`
	OpenRequest       *RequestRef `json:"open_request,omitempty"`
}

// Validation = hasil validasi terakhir; berlaku hanya untuk checksum yang sama.
type Validation struct {
	Checked   bool       `json:"checked"`
	OK        bool       `json:"ok"`
	Message   string     `json:"message,omitempty"`
	Checksum  string     `json:"-"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Current   bool       `json:"current"`
}

// RequestRef = ringkasan pengajuan yang masih berjalan pada sebuah migration.
type RequestRef struct {
	ID     int64  `json:"id"`
	Action string `json:"action"`
	Status string `json:"status"`
}

// Reversible true bila migration punya SQL down.
func (m Migration) Reversible() bool { return m.IrreversibleReason == "" }

// Locked true bila definisi sudah pernah dieksekusi.
func (m Migration) Locked() bool { return m.LockedAt != nil }

// Editable true bila SQL masih boleh diubah: belum pernah dieksekusi dan tidak
// sedang diajukan.
func (m Migration) Editable() bool { return !m.Locked() && m.OpenRequest == nil }

// ValidatedForCurrent true bila validasi terakhir lolos untuk revisi terbaru.
func (m Migration) ValidatedForCurrent() bool {
	return m.Validation.OK && m.Validation.Checksum == m.Checksum
}

func (m Migration) CreatedAtLabel() string {
	return m.CreatedAt.Format("02 Jan 2006 15:04")
}

// DefinitionChecksum mengikat SQL up, SQL down, dan alasan irreversible.
// Pemisah RS (0x1E) sama dengan yang dipakai migrasi data 0004.
func DefinitionChecksum(up, down, reason string) string {
	sum := sha256.Sum256([]byte(up + "\x1e" + down + "\x1e" + reason))
	return hex.EncodeToString(sum[:])
}

// VersionNow menghasilkan versi migration berbentuk YYYYMMDDHHMM.
func VersionNow(now time.Time) int64 {
	version, _ := strconv.ParseInt(now.Format("200601021504"), 10, 64)
	return version
}

// DeriveName mengambil nama dari komentar pertama SQL, atau statement pertama.
func DeriveName(sqlText string) string {
	var comment string
	for _, line := range strings.Split(sqlText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "--") {
			if comment == "" {
				comment = strings.TrimSpace(strings.TrimPrefix(trimmed, "--"))
			}
			continue
		}
		break
	}
	if comment != "" {
		return truncateName(comment)
	}
	for _, statement := range strings.Split(sqlText, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		return truncateName(statement)
	}
	return "Migration tanpa nama"
}

func truncateName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if len(name) <= maxNameLength {
		return name
	}
	return name[:maxNameLength] + "…"
}

// credentialPattern menangkap password di dalam connection string.
var credentialPattern = regexp.MustCompile(`(?i)((?:postgres(?:ql)?|mysql)://[^:\s/@]+:)[^@\s]+(@)`)

// sanitizePGError membersihkan pesan error dari kredensial sebelum disimpan
// di riwayat, dan memotongnya agar tidak membanjiri tampilan.
func sanitizePGError(err error) string {
	message := credentialPattern.ReplaceAllString(err.Error(), "${1}***${2}")
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > maxErrorLength {
		message = message[:maxErrorLength] + "…"
	}
	return message
}

// MigrationStore = penyimpanan definisi migration.
type MigrationStore interface {
	List(ctx context.Context, connID int64) ([]Migration, error)
	Get(ctx context.Context, id int64) (Migration, error)
	Create(ctx context.Context, connID int64, in MigrationInput, userID int64) (Migration, error)
	Update(ctx context.Context, id int64, in MigrationInput) (Migration, error)
	SaveValidation(ctx context.Context, id int64, checksum string, ok bool, message string) error
}

type PGMigrationStore struct {
	platform *pgxpool.Pool
}

func NewPGMigrationStore(platform *pgxpool.Pool) *PGMigrationStore {
	return &PGMigrationStore{platform: platform}
}

var _ MigrationStore = (*PGMigrationStore)(nil)

// migrationColumns memuat definisi, posisi dari eksekusi sukses terakhir,
// dan pengajuan yang masih berjalan dalam satu kueri.
const migrationColumns = `
       m.id, m.connection_id, m.version, m.name, m.up_sql, COALESCE(m.down_sql, ''),
       COALESCE(m.irreversible_reason, ''), m.revision, m.checksum, m.locked_at,
       m.validation_ok, COALESCE(m.validation_message, ''), COALESCE(m.validated_checksum, ''),
       m.validated_at, m.created_by, COALESCE(u.username, ''), m.created_at, m.updated_at,
       EXISTS (SELECT 1 FROM migration_executions e WHERE e.migration_id = m.id),
       COALESCE(last_ok.id, 0), COALESCE(last_ok.action, ''),
       COALESCE(open_req.id, 0), COALESCE(open_req.action, ''), COALESCE(open_req.status, '')`

const migrationJoins = `
      FROM migrations m
      LEFT JOIN users u ON u.id = m.created_by
      LEFT JOIN LATERAL (
            SELECT e.id, e.action FROM migration_executions e
             WHERE e.migration_id = m.id AND e.status = 'success'
             ORDER BY e.id DESC LIMIT 1) last_ok ON TRUE
      LEFT JOIN LATERAL (
            SELECT r.id, r.action, r.status FROM migration_requests r
             WHERE r.migration_id = m.id
               AND r.status IN ('pending_review', 'approved', 'queued', 'running')
             ORDER BY r.id DESC LIMIT 1) open_req ON TRUE`

func scanMigration(row pgx.Row) (Migration, error) {
	var m Migration
	var lockedAt, validatedAt sql.NullTime
	var validationOK sql.NullBool
	var createdBy sql.NullInt64
	var hasAttempts bool
	var lastOKID, openID int64
	var lastOKAction, openAction, openStatus string
	err := row.Scan(
		&m.ID, &m.ConnectionID, &m.Version, &m.Name, &m.UpSQL, &m.DownSQL,
		&m.IrreversibleReason, &m.Revision, &m.Checksum, &lockedAt,
		&validationOK, &m.Validation.Message, &m.Validation.Checksum,
		&validatedAt, &createdBy, &m.CreatedByName, &m.CreatedAt, &m.UpdatedAt,
		&hasAttempts, &lastOKID, &lastOKAction, &openID, &openAction, &openStatus,
	)
	if err != nil {
		return Migration{}, err
	}
	if lockedAt.Valid {
		m.LockedAt = &lockedAt.Time
	}
	if createdBy.Valid {
		m.CreatedBy = &createdBy.Int64
	}
	if validatedAt.Valid {
		m.Validation.CheckedAt = &validatedAt.Time
	}
	m.Validation.Checked = validationOK.Valid
	m.Validation.OK = validationOK.Valid && validationOK.Bool
	m.Validation.Current = m.Validation.Checked && m.Validation.Checksum == m.Checksum

	state := MigrationState{
		MigrationID:       m.ID,
		Reversible:        m.Reversible(),
		HasAttempts:       hasAttempts,
		LastSuccessID:     lastOKID,
		LastSuccessAction: lastOKAction,
	}
	m.State = state.State()
	m.ActiveExecutionID = state.ActiveExecutionID()
	if openID != 0 {
		m.OpenRequest = &RequestRef{ID: openID, Action: openAction, Status: openStatus}
	}
	return m, nil
}

func (s *PGMigrationStore) List(ctx context.Context, connID int64) ([]Migration, error) {
	rows, err := s.platform.Query(ctx,
		`SELECT `+migrationColumns+migrationJoins+`
		  WHERE m.connection_id = $1
		  ORDER BY m.version DESC, m.id DESC`, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Migration, 0, 8)
	for rows.Next() {
		m, err := scanMigration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *PGMigrationStore) Get(ctx context.Context, id int64) (Migration, error) {
	m, err := scanMigration(s.platform.QueryRow(ctx,
		`SELECT `+migrationColumns+migrationJoins+` WHERE m.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Migration{}, ErrMigrationNotFound
	}
	if err != nil {
		return Migration{}, err
	}
	return m, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Create menyimpan draft baru dengan versi YYYYMMDDHHMM. Bila versi sudah
// dipakai koneksi yang sama, versi naik satu langkah.
func (s *PGMigrationStore) Create(ctx context.Context, connID int64, in MigrationInput, userID int64) (Migration, error) {
	in, err := in.Normalize()
	if err != nil {
		return Migration{}, err
	}
	checksum := DefinitionChecksum(in.UpSQL, in.DownSQL, in.IrreversibleReason)
	// Versi minimal versi terakhir + 1, agar banyak create dalam satu menit tetap unik.
	for attempt := 0; attempt < 5; attempt++ {
		var id int64
		err := s.platform.QueryRow(ctx,
			`INSERT INTO migrations (connection_id, version, name, up_sql, down_sql,
			                         irreversible_reason, checksum, created_by)
			 SELECT $1, GREATEST($2::bigint, COALESCE(max(version), 0) + 1), $3, $4, $5, $6, $7, $8
			   FROM migrations WHERE connection_id = $1
			 ON CONFLICT (connection_id, version) DO NOTHING
			 RETURNING id`,
			connID, VersionNow(time.Now()), in.Name, in.UpSQL, nullable(in.DownSQL),
			nullable(in.IrreversibleReason), checksum, userID).Scan(&id)
		if err == nil {
			return s.Get(ctx, id)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Migration{}, err
		}
	}
	return Migration{}, ErrVersionConflict
}

// Update mengganti SQL draft, menaikkan revisi, dan menghapus hasil validasi
// lama. Ditolak bila migration sudah pernah dieksekusi atau sedang diajukan.
func (s *PGMigrationStore) Update(ctx context.Context, id int64, in MigrationInput) (Migration, error) {
	in, err := in.Normalize()
	if err != nil {
		return Migration{}, err
	}
	current, err := s.Get(ctx, id)
	if err != nil {
		return Migration{}, err
	}
	if current.Locked() {
		return Migration{}, ErrMigrationLocked
	}
	if current.OpenRequest != nil {
		return Migration{}, ErrMigrationInReview
	}
	checksum := DefinitionChecksum(in.UpSQL, in.DownSQL, in.IrreversibleReason)
	if checksum == current.Checksum && in.Name == current.Name {
		return current, nil
	}
	revision := current.Revision
	if checksum != current.Checksum {
		revision++
	}
	tag, err := s.platform.Exec(ctx,
		`UPDATE migrations
		    SET name = $2, up_sql = $3, down_sql = $4, irreversible_reason = $5,
		        checksum = $6, revision = $7, updated_at = now(),
		        validation_ok = CASE WHEN $6 = checksum THEN validation_ok END,
		        validation_message = CASE WHEN $6 = checksum THEN validation_message END,
		        validated_checksum = CASE WHEN $6 = checksum THEN validated_checksum END,
		        validated_at = CASE WHEN $6 = checksum THEN validated_at END
		  WHERE id = $1 AND locked_at IS NULL
		    AND NOT EXISTS (SELECT 1 FROM migration_requests r
		                     WHERE r.migration_id = $1
		                       AND r.status IN ('pending_review', 'approved', 'queued', 'running'))`,
		id, in.Name, in.UpSQL, nullable(in.DownSQL), nullable(in.IrreversibleReason),
		checksum, revision)
	if err != nil {
		return Migration{}, err
	}
	if tag.RowsAffected() == 0 {
		return Migration{}, ErrMigrationInReview
	}
	return s.Get(ctx, id)
}

// SaveValidation menyimpan hasil validasi untuk checksum tertentu.
func (s *PGMigrationStore) SaveValidation(ctx context.Context, id int64, checksum string, ok bool, message string) error {
	tag, err := s.platform.Exec(ctx,
		`UPDATE migrations
		    SET validation_ok = $3, validation_message = $4,
		        validated_checksum = $2, validated_at = now()
		  WHERE id = $1 AND checksum = $2`, id, checksum, ok, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrChecksumMismatch
	}
	return nil
}

// loadStates memuat posisi seluruh migration pada sebuah koneksi untuk
// pemeriksaan linear (PlanAction).
func loadStates(ctx context.Context, q pgxQuerier, connID int64) ([]MigrationState, error) {
	rows, err := q.Query(ctx,
		`SELECT m.id, m.down_sql IS NOT NULL,
		        EXISTS (SELECT 1 FROM migration_executions e WHERE e.migration_id = m.id),
		        COALESCE(last_ok.id, 0), COALESCE(last_ok.action, '')
		   FROM migrations m
		   LEFT JOIN LATERAL (
		         SELECT e.id, e.action FROM migration_executions e
		          WHERE e.migration_id = m.id AND e.status = 'success'
		          ORDER BY e.id DESC LIMIT 1) last_ok ON TRUE
		  WHERE m.connection_id = $1`, connID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MigrationState
	for rows.Next() {
		var s MigrationState
		if err := rows.Scan(&s.MigrationID, &s.Reversible, &s.HasAttempts, &s.LastSuccessID, &s.LastSuccessAction); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// pgxQuerier dipenuhi *pgxpool.Pool maupun pgx.Tx.
type pgxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
