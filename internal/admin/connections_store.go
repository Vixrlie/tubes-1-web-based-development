package admin

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
)

var (
	ErrConnectionNotFound  = errors.New("koneksi database tidak ditemukan")
	ErrConnectionNameTaken = errors.New("nama koneksi sudah dipakai")
	ErrNameRequired        = errors.New("Nama tampilan wajib diisi")
	ErrHostRequired        = errors.New("Host wajib diisi")
	ErrPortInvalid         = errors.New("Port harus berupa angka antara 1 dan 65535")
	ErrDatabaseRequired    = errors.New("Nama database wajib diisi")
	ErrUsernameRequired    = errors.New("Username database wajib diisi")
	ErrPasswordRequired    = errors.New("Password database wajib diisi")
)

// ConnectionRow adalah data koneksi untuk halaman admin. Password dan
// connection string tidak pernah ikut di struct ini.
type ConnectionRow struct {
	ID          int64
	Name        string
	Description string
	Host        string
	Port        string
	Database    string
	Username    string
	CreatedAt   time.Time
}

type ConnectionInput struct {
	Name        string
	Description string
	Host        string
	Port        string
	Database    string
	Username    string
	Password    string
}

type ConnectionStore interface {
	List(ctx context.Context) ([]ConnectionRow, error)
	Get(ctx context.Context, id int64) (ConnectionRow, error)
	Create(ctx context.Context, in ConnectionInput, createdBy int64) (ConnectionRow, error)
	Update(ctx context.Context, id int64, in ConnectionInput) (ConnectionRow, error)
	Delete(ctx context.Context, id int64) error
	// Test membuka koneksi ke tujuan lalu menjalankan SELECT 1.
	Test(ctx context.Context, id int64) error
	// ConnectionGroups mengembalikan group yang punya akses per koneksi
	// (tabel daftar & kartu "Group dengan Akses" format fe-components).
	ConnectionGroups(ctx context.Context) (map[int64][]web.ConnectionGroup, error)
}

type PGConnectionStore struct {
	pool *pgxpool.Pool
}

var _ ConnectionStore = (*PGConnectionStore)(nil)

func NewPGConnectionStore(pool *pgxpool.Pool) *PGConnectionStore {
	return &PGConnectionStore{pool: pool}
}

const connectionCols = `id, name, COALESCE(description, ''), connection_url, created_at`

func (s *PGConnectionStore) List(ctx context.Context) ([]ConnectionRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+connectionCols+` FROM db_connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ConnectionRow
	for rows.Next() {
		row, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *PGConnectionStore) Get(ctx context.Context, id int64) (ConnectionRow, error) {
	return scanConnection(s.pool.QueryRow(ctx, `SELECT `+connectionCols+` FROM db_connections WHERE id = $1`, id))
}

func (s *PGConnectionStore) Create(ctx context.Context, in ConnectionInput, createdBy int64) (ConnectionRow, error) {
	rawURL, err := buildConnectionURL(in, "")
	if err != nil {
		return ConnectionRow{}, err
	}
	row := s.pool.QueryRow(ctx,
		`INSERT INTO db_connections (name, description, connection_url, created_by)
		 VALUES ($1, NULLIF($2, ''), $3, $4)
		 RETURNING `+connectionCols,
		in.Name, in.Description, rawURL, createdBy)
	c, err := scanConnection(row)
	if isNameConflict(err) {
		return ConnectionRow{}, ErrConnectionNameTaken
	}
	return c, err
}

func (s *PGConnectionStore) Update(ctx context.Context, id int64, in ConnectionInput) (ConnectionRow, error) {
	// Password write-only: kosong berarti pakai password yang tersimpan.
	var currentURL string
	err := s.pool.QueryRow(ctx, `SELECT connection_url FROM db_connections WHERE id = $1`, id).Scan(&currentURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionRow{}, ErrConnectionNotFound
	}
	if err != nil {
		return ConnectionRow{}, err
	}

	rawURL, err := buildConnectionURL(in, currentURL)
	if err != nil {
		return ConnectionRow{}, err
	}
	row := s.pool.QueryRow(ctx,
		`UPDATE db_connections
		    SET name = $2, description = NULLIF($3, ''), connection_url = $4, updated_at = now()
		  WHERE id = $1
		  RETURNING `+connectionCols,
		id, in.Name, in.Description, rawURL)
	c, err := scanConnection(row)
	if isNameConflict(err) {
		return ConnectionRow{}, ErrConnectionNameTaken
	}
	return c, err
}

func (s *PGConnectionStore) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM db_connections WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

func (s *PGConnectionStore) Test(ctx context.Context, id int64) error {
	var rawURL string
	err := s.pool.QueryRow(ctx, `SELECT connection_url FROM db_connections WHERE id = $1`, id).Scan(&rawURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConnectionNotFound
	}
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := pgx.Connect(ctx, rawURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var one int
	return conn.QueryRow(ctx, "SELECT 1").Scan(&one)
}

func scanConnection(row pgx.Row) (ConnectionRow, error) {
	var c ConnectionRow
	var rawURL string
	err := row.Scan(&c.ID, &c.Name, &c.Description, &rawURL, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConnectionRow{}, ErrConnectionNotFound
	}
	if err != nil {
		return ConnectionRow{}, err
	}
	c.Host, c.Port, c.Database, c.Username = parseConnectionURL(rawURL)
	return c, nil
}

// buildConnectionURL menyusun connection string dari isi formulir. Password
// kosong saat ubah berarti password lama dipertahankan.
func buildConnectionURL(in ConnectionInput, currentURL string) (string, error) {
	password := in.Password
	if password == "" && currentURL != "" {
		password = passwordFromURL(currentURL)
	}
	if password == "" {
		return "", ErrPasswordRequired
	}

	target := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(in.Username, password),
		Host:   net.JoinHostPort(in.Host, in.Port),
		Path:   "/" + in.Database,
	}
	query := target.Query()
	query.Set("sslmode", "disable")
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func parseConnectionURL(rawURL string) (host, port, database, username string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", ""
	}
	port = parsed.Port()
	if port == "" {
		port = "5432"
	}
	if parsed.User != nil {
		username = parsed.User.Username()
	}
	return parsed.Hostname(), port, strings.TrimPrefix(parsed.Path, "/"), username
}

func passwordFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User == nil {
		return ""
	}
	password, _ := parsed.User.Password()
	return password
}

// ValidateConnection memeriksa isi formulir. Password hanya wajib saat membuat.
func ValidateConnection(in ConnectionInput, requirePassword bool) map[string]string {
	errorsByField := map[string]string{}
	if strings.TrimSpace(in.Name) == "" {
		errorsByField["name"] = ErrNameRequired.Error()
	}
	if strings.TrimSpace(in.Host) == "" {
		errorsByField["host"] = ErrHostRequired.Error()
	}
	port, err := strconv.Atoi(strings.TrimSpace(in.Port))
	if err != nil || port < 1 || port > 65535 {
		errorsByField["port"] = ErrPortInvalid.Error()
	}
	if strings.TrimSpace(in.Database) == "" {
		errorsByField["database"] = ErrDatabaseRequired.Error()
	}
	if strings.TrimSpace(in.Username) == "" {
		errorsByField["username"] = ErrUsernameRequired.Error()
	}
	if requirePassword && in.Password == "" {
		errorsByField["password"] = ErrPasswordRequired.Error()
	}
	return errorsByField
}

// isNameConflict menandai pelanggaran unique pada nama koneksi.
func isNameConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "db_connections_name_key"
}

// ConnectionGroups mengembalikan group yang punya akses per koneksi, urut nama
// group: dipakai tabel daftar (nama group per koneksi) dan kartu detail.
func (s *PGConnectionStore) ConnectionGroups(ctx context.Context) (map[int64][]web.ConnectionGroup, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT g.connection_id, gr.name, COUNT(m.user_id), g.level
		   FROM grants g
		   JOIN groups gr ON gr.id = g.group_id
		   LEFT JOIN memberships m ON m.group_id = gr.id
		  GROUP BY g.connection_id, gr.name, g.level
		  ORDER BY gr.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64][]web.ConnectionGroup{}
	for rows.Next() {
		var connID int64
		var grp web.ConnectionGroup
		var level string
		if err := rows.Scan(&connID, &grp.Name, &grp.MemberCount, &level); err != nil {
			return nil, err
		}
		grp.Level = level
		out[connID] = append(out[connID], grp)
	}
	return out, rows.Err()
}
