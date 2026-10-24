package dbm

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
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrMigrationNotFound = errors.New("migration tidak ditemukan")
	ErrMigrationNotDraft = errors.New("migration sudah tidak berstatus draft")
	ErrChecksumMismatch  = errors.New("checksum SQL tidak cocok")
	ErrVersionConflict   = errors.New("versi migration bentrok")
	ErrEmptySQL          = errors.New("SQL migration kosong")
)

const (
	StatusDraft   = "draft"
	StatusApplied = "applied"
	StatusFailed  = "failed"
)

const maxNameLength = 80

const maxErrorLength = 2000

type Migration struct {
	ID           int64
	ConnectionID int64
	Version      int64
	SQL          string
	Checksum     string
	Status       string
	Error        string
	CreatedBy    *int64
	CreatedAt    time.Time
	ExecutedBy   *int64
	ExecutedAt   *time.Time

	CreatedByName  string
	ExecutedByName string
}

func (m Migration) Name() string {
	return DeriveName(m.SQL)
}

func (m Migration) CreatedAtLabel() string {
	return m.CreatedAt.Format("02 Jan 2006 15:04")
}

// ExecutedAtLabel waktu eksekusi migration, atau keterangan bila belum dijalankan.
func (m Migration) ExecutedAtLabel() string {
	if m.ExecutedAt == nil {
		return "belum dijalankan"
	}
	return m.ExecutedAt.Format("02 Jan 2006 15:04")
}

func Checksum(sqlText string) string {
	sum := sha256.Sum256([]byte(sqlText))
	return hex.EncodeToString(sum[:])
}

// VersionNow menghasilkan versi migration berbentuk YYYYMMDDHHMM.
func VersionNow(now time.Time) int64 {
	version, _ := strconv.ParseInt(now.Format("200601021504"), 10, 64)
	return version
}

func InjectName(sqlText, name string) string {
	if strings.TrimSpace(name) == "" {
		return sqlText
	}
	lines := strings.Split(sqlText, "\n")
	var body []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			body = append(body, line)
			continue
		}
		if strings.HasPrefix(trimmed, "--") {
			// Ganti komentar pertama, pertahankan baris kosong/indentasi sebelumnya.
			if i == 0 || (i > 0 && strings.TrimSpace(lines[i-1]) == "") {
				body = append(body, "-- "+name)
				body = append(body, lines[i+1:]...)
				return strings.Join(body, "\n")
			}
		}
		break
	}
	return "-- " + name + "\n" + sqlText
}

// DeriveName mengambil nama migration dari SQL: komentar pertama, atau statement pertama bila tidak ada komentar.
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

type MigrationStore interface {
	List(ctx context.Context, connID int64) ([]Migration, error)
	Get(ctx context.Context, id int64) (Migration, error)
	CreateDraft(ctx context.Context, connID int64, sqlText string, userID int64) (Migration, error)
	UpdateDraft(ctx context.Context, id int64, sqlText string) error
}

type PGMigrationStore struct {
	platform *pgxpool.Pool
}

func NewPGMigrationStore(platform *pgxpool.Pool) *PGMigrationStore {
	return &PGMigrationStore{platform: platform}
}

var _ MigrationStore = (*PGMigrationStore)(nil)

const migrationColumns = `
       m.id, m.connection_id, m.version, m.sql, m.checksum, m.status,
       COALESCE(m.error, ''), m.created_by, m.created_at, m.executed_by, m.executed_at,
       COALESCE(u_creator.username, ''), COALESCE(u_executor.username, '')`

const migrationJoins = `
      FROM migration_history m
      LEFT JOIN users u_creator ON u_creator.id = m.created_by
      LEFT JOIN users u_executor ON u_executor.id = m.executed_by`

func scanMigration(row pgx.Row) (Migration, error) {
	var m Migration
	var createdBy sql.NullInt64
	var executedBy sql.NullInt64
	var executedAt sql.NullTime
	err := row.Scan(
		&m.ID, &m.ConnectionID, &m.Version, &m.SQL, &m.Checksum, &m.Status,
		&m.Error, &createdBy, &m.CreatedAt, &executedBy, &executedAt,
		&m.CreatedByName, &m.ExecutedByName,
	)
	if err != nil {
		return Migration{}, err
	}
	if createdBy.Valid {
		m.CreatedBy = &createdBy.Int64
	}
	if executedBy.Valid {
		m.ExecutedBy = &executedBy.Int64
	}
	if executedAt.Valid {
		m.ExecutedAt = &executedAt.Time
	}
	return m, nil
}

func (s *PGMigrationStore) List(ctx context.Context, connID int64) ([]Migration, error) {
	rows, err := s.platform.Query(ctx,
		`SELECT `+migrationColumns+migrationJoins+`
		  WHERE m.connection_id = $1
		  ORDER BY m.version DESC`, connID)
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
		`SELECT `+migrationColumns+migrationJoins+`
		  WHERE m.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Migration{}, ErrMigrationNotFound
	}
	if err != nil {
		return Migration{}, err
	}
	return m, nil
}

// CreateDraft menyimpan SQL sebagai draft baru dengan versi YYYYMDDHHMM.
// Bila versi sudah dipakai connection yang sama, versi naik satu langkah.
func (s *PGMigrationStore) CreateDraft(ctx context.Context, connID int64, sqlText string, userID int64) (Migration, error) {
	checksum := Checksum(sqlText)
	version := VersionNow(time.Now())
	for attempt := 0; attempt < 5; attempt++ {
		m, err := scanMigration(s.platform.QueryRow(ctx,
			`INSERT INTO migration_history (connection_id, version, sql, checksum, status, created_by)
			      VALUES ($1, $2, $3, $4, 'draft', $5)
			 ON CONFLICT (connection_id, version) DO NOTHING
			 RETURNING id, connection_id, version, sql, checksum, status,
			           COALESCE(error, ''), created_by, created_at, executed_by, executed_at, '', ''`,
			connID, version, sqlText, checksum, userID))
		if err == nil {
			return m, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Migration{}, err
		}
		version++
	}
	return Migration{}, ErrVersionConflict
}

// UpdateDraft mengganti SQL draft dan menghitung ulang checksum-nya.
// Migration yang sudah applied atau failed tidak bisa diubah.
func (s *PGMigrationStore) UpdateDraft(ctx context.Context, id int64, sqlText string) error {
	tag, err := s.platform.Exec(ctx,
		`UPDATE migration_history
		    SET sql = $2, checksum = $3
		  WHERE id = $1 AND status = 'draft'`, id, sqlText, Checksum(sqlText))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMigrationNotDraft
	}
	return nil
}

// MigrationRunner menjalankan SQL migration pada database target dalam satu
// transaksi, lalu mencatat hasilnya di database platform.
type MigrationRunner struct {
	platform *pgxpool.Pool
	registry Registry
}

func NewMigrationRunner(platform *pgxpool.Pool, registry Registry) *MigrationRunner {
	return &MigrationRunner{platform: platform, registry: registry}
}

// credentialPattern menangkap password di dalam connection string PostgreSQL.
var credentialPattern = regexp.MustCompile(`(?i)(postgres(?:ql)?://[^:\s/@]+:)[^@\s]+(@)`)

// sanitizePGError membersihkan pesan error PostgreSQL dari kredensial sebelum
// disimpan di riwayat, dan memotongnya agar tidak membanjiri halaman.
func sanitizePGError(err error) string {
	message := credentialPattern.ReplaceAllString(err.Error(), "${1}***${2}")
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > maxErrorLength {
		message = message[:maxErrorLength] + "…"
	}
	return message
}

func (r *MigrationRunner) Execute(ctx context.Context, connID, migrationID, userID int64) (string, error) {
	platformTx, err := r.platform.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer platformTx.Rollback(ctx)

	var m Migration
	err = platformTx.QueryRow(ctx,
		`SELECT id, connection_id, version, sql, checksum, status
		   FROM migration_history
		  WHERE id = $1
		  FOR UPDATE`, migrationID).
		Scan(&m.ID, &m.ConnectionID, &m.Version, &m.SQL, &m.Checksum, &m.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrMigrationNotFound
	}
	if err != nil {
		return "", err
	}
	if m.ConnectionID != connID {
		return "", ErrMigrationNotFound
	}
	if m.Status != StatusDraft {
		return "", ErrMigrationNotDraft
	}
	if m.Checksum != Checksum(m.SQL) {
		return "", ErrChecksumMismatch
	}

	pool, err := r.registry.Get(ctx, connID)
	if err != nil {
		return "", err
	}

	if targetErr := runOnTarget(ctx, pool, connID, m.SQL); targetErr != nil {
		message := sanitizePGError(targetErr)
		if _, err := platformTx.Exec(ctx,
			`UPDATE migration_history
			    SET status = 'failed', error = $2, executed_by = $3, executed_at = now()
			  WHERE id = $1`, m.ID, message, userID); err != nil {
			return "", err
		}
		if err := platformTx.Commit(ctx); err != nil {
			return "", err
		}
		return StatusFailed, nil
	}

	if _, err := platformTx.Exec(ctx,
		`UPDATE migration_history
		    SET status = 'applied', error = NULL, executed_by = $2, executed_at = now()
		  WHERE id = $1`, m.ID, userID); err != nil {
		return "", err
	}
	if err := platformTx.Commit(ctx); err != nil {
		return "", err
	}
	return StatusApplied, nil
}

// runOnTarget menjalankan SQL di database target dalam satu transaksi.
// Advisory lock mencegah dua eksekusi bersamaan pada koneksi yang sama.
func runOnTarget(ctx context.Context, pool *pgxpool.Pool, connID int64, sqlText string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, connID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, sqlText); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
