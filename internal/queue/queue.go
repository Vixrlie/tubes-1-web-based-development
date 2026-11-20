// Package queue: antrian job migration di tabel migration_jobs. Workflow hanya
// bergantung pada dbm.Enqueuer, jadi queue bisa diganti broker lain.
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/propagation"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
)

var (
	// ErrBusy: sumber daya (lock database) sedang dipakai. Job dijadwalkan
	// ulang tanpa menghabiskan jatah attempt.
	ErrBusy = errors.New("queue: resource busy")
	// ErrRetry dibungkus oleh handler untuk kegagalan sementara.
	ErrRetry = errors.New("queue: retry")
)

// Retry menandai err sebagai kegagalan sementara.
func Retry(err error) error { return fmt.Errorf("%w: %w", ErrRetry, err) }

// Busy membungkus err sebagai ErrBusy: job dijadwalkan ulang tanpa
// menghabiskan attempt.
func Busy(err error) error { return fmt.Errorf("%w: %w", ErrBusy, err) }

// Job = satu baris migration_jobs yang sedang diproses.
type Job struct {
	ID           int64
	RequestID    int64
	ConnectionID int64
	Attempts     int // termasuk attempt saat ini
	MaxAttempts  int
}

// Handler memproses job. nil = selesai; ErrBusy/ErrRetry = coba lagi nanti;
// galat lain = gagal permanen.
type Handler func(ctx context.Context, job Job) error

// Queue mempublikasikan job.
type Queue struct {
	pool        *pgxpool.Pool
	MaxAttempts int
}

func New(pool *pgxpool.Pool) *Queue { return &Queue{pool: pool, MaxAttempts: 3} }

var propagator = propagation.TraceContext{}

// Enqueue idempotent per request: job yang sudah ada dikembalikan ID-nya.
func (q *Queue) Enqueue(ctx context.Context, tx pgx.Tx, requestID, connID int64) (int64, error) {
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	traceJSON, err := json.Marshal(carrier)
	if err != nil {
		return 0, err
	}
	max := q.MaxAttempts
	if max <= 0 {
		max = 3
	}
	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO migration_jobs (request_id, connection_id, max_attempts, trace_context)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (request_id) DO UPDATE SET updated_at = migration_jobs.updated_at
		 RETURNING id`, requestID, connID, max, traceJSON).Scan(&id)
	return id, err
}

// Stats = jumlah job per status (untuk API/metrics).
func (q *Queue) Stats(ctx context.Context) (map[string]int64, error) {
	rows, err := q.pool.Query(ctx, `SELECT status, COUNT(*) FROM migration_jobs GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{"pending": 0, "running": 0, "done": 0, "failed": 0}
	for rows.Next() {
		var s string
		var n int64
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// Worker mengambil job secara berkala dan menjalankannya dengan Handler.
type Worker struct {
	pool        *pgxpool.Pool
	handler     Handler
	logger      *activity.Logger
	ID          string
	Concurrency int
	Poll        time.Duration
	// Lease = batas waktu tanpa heartbeat sebelum job dianggap ditinggalkan.
	Lease time.Duration
	// Backoff dasar retry; berlipat dua tiap attempt (maks 1 menit).
	Backoff time.Duration
	// OnGiveUp dipanggil saat job gagal permanen (attempt habis atau galat
	// non-retry) agar pemilik job dapat menandai pekerjaannya gagal.
	OnGiveUp func(ctx context.Context, job Job, err error)
}

func NewWorker(pool *pgxpool.Pool, handler Handler, logger *activity.Logger) *Worker {
	host, _ := os.Hostname()
	return &Worker{
		pool:        pool,
		handler:     handler,
		logger:      logger,
		ID:          host + "-" + strconv.Itoa(os.Getpid()),
		Concurrency: 4,
		Poll:        500 * time.Millisecond,
		Lease:       30 * time.Second,
		Backoff:     2 * time.Second,
	}
}

// Run memproses job sampai ctx dibatalkan.
func (w *Worker) Run(ctx context.Context) {
	n := w.Concurrency
	if n <= 0 {
		n = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			w.loop(ctx, slot)
		}(i)
	}
	wg.Wait()
}

func (w *Worker) loop(ctx context.Context, slot int) {
	ticker := time.NewTicker(w.Poll)
	defer ticker.Stop()
	for {
		// Habiskan job yang tersedia sebelum menunggu tick berikutnya.
		for {
			if ctx.Err() != nil {
				return
			}
			processed, err := w.ProcessOne(ctx)
			if err != nil && ctx.Err() == nil {
				w.log(ctx, "queue.worker.error", activity.StatusFailed, map[string]string{"slot": strconv.Itoa(slot)}, err)
				break
			}
			if !processed {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// claimSQL juga mengambil alih job yang lease-nya habis, dan melewati database
// yang sedang punya job running.
const claimSQL = `
UPDATE migration_jobs j
   SET status = 'running', attempts = j.attempts + 1, locked_by = $1,
       locked_at = now(), updated_at = now()
 WHERE j.id = (
       SELECT c.id FROM migration_jobs c
        WHERE ((c.status = 'pending' AND c.available_at <= now())
           OR (c.status = 'running' AND c.locked_at < now() - $2::interval))
          AND NOT EXISTS (
              SELECT 1 FROM migration_jobs o
               WHERE o.connection_id = c.connection_id AND o.id <> c.id
                 AND o.status = 'running' AND o.locked_at >= now() - $2::interval)
        ORDER BY c.available_at, c.id
        FOR UPDATE SKIP LOCKED
        LIMIT 1)
RETURNING j.id, j.request_id, j.connection_id, j.attempts, j.max_attempts, j.trace_context,
          extract(epoch FROM (now() - j.created_at))`

// ProcessOne mengambil dan memproses satu job. processed=false bila antrian kosong.
func (w *Worker) ProcessOne(ctx context.Context) (processed bool, err error) {
	var job Job
	var traceJSON []byte
	var waitSeconds float64
	lease := w.Lease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	err = w.pool.QueryRow(ctx, claimSQL, w.ID, lease.String()).Scan(
		&job.ID, &job.RequestID, &job.ConnectionID, &job.Attempts, &job.MaxAttempts, &traceJSON, &waitSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	carrier := propagation.MapCarrier{}
	_ = json.Unmarshal(traceJSON, &carrier)
	jobCtx := propagator.Extract(ctx, carrier)
	jobCtx, span := activity.StartSpan(jobCtx, "queue.job")
	detail := map[string]string{
		"job_id": strconv.FormatInt(job.ID, 10), "request_id": strconv.FormatInt(job.RequestID, 10),
		"database_id": strconv.FormatInt(job.ConnectionID, 10), "attempt": strconv.Itoa(job.Attempts),
		"queue_wait_ms": strconv.FormatInt(int64(waitSeconds*1000), 10), "worker": w.ID,
	}
	w.log(jobCtx, "queue.job.claimed", "started", detail, nil)

	stop := w.heartbeat(jobCtx, job.ID, lease)
	handlerErr := w.safeHandle(jobCtx, job)
	stop()

	err = w.complete(ctx, job, handlerErr, detail)
	activity.EndSpan(span, handlerErr != nil && !errors.Is(handlerErr, ErrBusy))
	return true, err
}

func (w *Worker) safeHandle(ctx context.Context, job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return w.handler(ctx, job)
}

// heartbeat memperbarui locked_at selama handler berjalan agar job panjang
// tidak diambil worker lain.
func (w *Worker) heartbeat(ctx context.Context, jobID int64, lease time.Duration) func() {
	hbCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(lease / 3)
		defer t.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-t.C:
				_, _ = w.pool.Exec(hbCtx,
					`UPDATE migration_jobs SET locked_at = now() WHERE id = $1 AND locked_by = $2 AND status = 'running'`,
					jobID, w.ID)
			}
		}
	}()
	return func() { cancel(); <-done }
}

func (w *Worker) backoff(attempt int) time.Duration {
	base := w.Backoff
	if base <= 0 {
		base = 2 * time.Second
	}
	d := base << max(attempt-1, 0)
	if d > time.Minute || d <= 0 {
		d = time.Minute
	}
	return d
}

// complete menyimpan hasil job.
func (w *Worker) complete(ctx context.Context, job Job, handlerErr error, detail map[string]string) error {
	// Simpan hasil walau ctx sudah dibatalkan (shutdown) agar job tidak menggantung.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	switch {
	case handlerErr == nil:
		w.log(ctx, "queue.job.done", activity.StatusSuccess, detail, nil)
		_, err := w.pool.Exec(ctx,
			`UPDATE migration_jobs SET status = 'done', last_error = NULL, locked_by = NULL, updated_at = now()
			  WHERE id = $1`, job.ID)
		return err

	case errors.Is(handlerErr, ErrBusy):
		// Tidak dihitung sebagai attempt.
		_, err := w.pool.Exec(ctx,
			`UPDATE migration_jobs SET status = 'pending', attempts = attempts - 1, locked_by = NULL,
			        available_at = now() + interval '1 second', updated_at = now()
			  WHERE id = $1`, job.ID)
		return err

	case errors.Is(handlerErr, ErrRetry) && job.Attempts < job.MaxAttempts:
		delay := w.backoff(job.Attempts)
		detail["retry_in"] = delay.String()
		w.log(ctx, "queue.job.retry", activity.StatusWarning, detail, handlerErr)
		_, err := w.pool.Exec(ctx,
			`UPDATE migration_jobs SET status = 'pending', last_error = $2, locked_by = NULL,
			        available_at = now() + $3::interval, updated_at = now()
			  WHERE id = $1`, job.ID, handlerErr.Error(), delay.String())
		return err

	default:
		w.log(ctx, "queue.job.failed", activity.StatusFailed, detail, handlerErr)
		if w.OnGiveUp != nil {
			w.OnGiveUp(ctx, job, handlerErr)
		}
		_, err := w.pool.Exec(ctx,
			`UPDATE migration_jobs SET status = 'failed', last_error = $2, locked_by = NULL, updated_at = now()
			  WHERE id = $1`, job.ID, handlerErr.Error())
		return err
	}
}

func (w *Worker) log(ctx context.Context, action, status string, detail map[string]string, err error) {
	w.logger.Log(ctx, activity.Event{UserID: "worker", Action: action, Status: status, Detail: detail, Err: err})
}
