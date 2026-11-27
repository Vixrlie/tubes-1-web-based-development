package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/access"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/activity"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/admin"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/config"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/db"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/dbm"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/queue"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/web"
	"github.com/Vixrlie/tubes-1-web-based-development/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	workerOnly := flag.Bool("worker-only", false, "jalankan hanya worker migration asynchronous tanpa HTTP server")
	flag.Parse()

	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logFile := os.Getenv("ACTIVITY_LOG_FILE")
	if logFile == "" {
		logFile = "observability/logs/activity.jsonl"
	}
	closeActivityLog, err := activity.ConfigureLogFile(logFile)
	if err != nil {
		return err
	}
	defer func() { _ = closeActivityLog() }()
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); endpoint != "" {
		shutdown, err := activity.StartTracing(ctx, endpoint)
		if err != nil {
			return err
		}
		defer func() {
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdown(flushCtx); err != nil {
				log.Printf("flush traces: %v", err)
			}
		}()
	}

	pool, err := db.NewPool(ctx, cfg.PlatformDBURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool, migrations.FS); err != nil {
		return err
	}

	userStore := auth.NewPGStore(pool)
	if err := auth.SeedAdmin(ctx, userStore, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return err
	}

	sessions, err := session.NewManager(session.NewPGStore(pool), session.Config{
		IdleTimeout:     cfg.SessionIdle,
		AbsoluteTimeout: cfg.SessionAbsolute,
		RotateInterval:  cfg.SessionRotate,
		GracePeriod:     cfg.SessionGrace,
	}, nil)
	if err != nil {
		return err
	}

	renderer, err := web.NewRenderer()
	if err != nil {
		return err
	}

	authHandlers := auth.NewHandlers(userStore, sessions, renderer, cfg.CookieSecure)
	activityLogger := activity.New()
	authHandlers.SetLogger(activityLogger)

	adminUsers := admin.NewPGUserStore(pool)
	accessPolicy := access.NewPolicy(access.NewPGLevelSource(pool))
	registry := dbm.NewURLRegistry(pool)
	defer registry.Close()
	connStore := admin.NewPGConnectionStore(pool)
	groupStore := admin.NewPGGroupStore(pool)
	connLister := admin.NewPGConnectionLister(pool)
	migrationStore := dbm.NewPGMigrationStore(pool)
	metrics := &activity.Metrics{}
	jobs := queue.New(pool)
	workflow := dbm.NewWorkflow(dbm.WorkflowConfig{
		Pool:     pool,
		Policy:   accessPolicy,
		Store:    migrationStore,
		Registry: registry,
		Queue:    jobs,
		Logger:   activityLogger,
	})
	dbmHandlers := dbm.NewHandlers(dbm.Config{
		Platform:   pool,
		Policy:     accessPolicy,
		Registry:   registry,
		Renderer:   renderer,
		Migrations: migrationStore,
		Workflow:   workflow,
	})

	// Worker eksekusi migration berjalan di proses yang sama, terpisah dari
	// request HTTP. Ganti dengan worker eksternal cukup lewat tabel migration_jobs.
	executor := dbm.NewExecutor(pool, registry, activityLogger, metrics)
	runJob := executor.QueueHandler(queue.Busy, queue.Retry)
	worker := queue.NewWorker(pool, func(ctx context.Context, job queue.Job) error {
		return runJob(ctx, dbm.ExecJob{
			JobID: job.ID, RequestID: job.RequestID,
			Attempt: job.Attempts, MaxAttempts: job.MaxAttempts,
		})
	}, activityLogger)
	worker.OnGiveUp = func(ctx context.Context, job queue.Job, err error) {
		if abandonErr := executor.Abandon(ctx, job.RequestID, err); abandonErr != nil {
			log.Printf("abandon request %d: %v", job.RequestID, abandonErr)
		}
	}
	// -worker-only: proses ini hanya menjalankan worker (tanpa HTTP), sehingga
	// worker dapat diskalakan terpisah; lock per database berlaku lintas proses.
	if *workerOnly {
		worker.Run(ctx)
		return nil
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.Run(ctx)
	}()
	defer func() {
		stop()
		<-workerDone
	}()

	devMode := os.Getenv("DEV_COMPONENTS") == "1"
	legacyHTML := os.Getenv("LEGACY_HTML") == "1"
	r := web.NewRouter()
	if os.Getenv("OBSERVABILITY_METRICS") == "1" {
		r.Get("/metrics", metrics.Handler(pool, registry.PoolStats))
	}
	if legacyHTML {
		authHandlers.PublicRoutes(r)
	}
	authHandlers.JSONRoutes(r, auth.NewSessionValidator(sessions))
	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://127.0.0.1:3100"
	}
	activityReader, err := activity.NewLokiReader(lokiURL)
	if err != nil {
		return err
	}
	admin.ActivityJSONRoutes(r, activityReader, auth.NewSessionValidator(sessions), cfg.CookieSecure)
	admin.RegisterAdminJSONRoutes(r, admin.AdminAPIConfig{
		Users:        adminUsers,
		Groups:       groupStore,
		Connections:  connStore,
		ConnLister:   connLister,
		Revoker:      sessions,
		Evictor:      registry,
		Validator:    auth.NewSessionValidator(sessions),
		CookieSecure: cfg.CookieSecure,
		Logger:       activityLogger,
	})
	if devMode {
		web.DevRoutes{R: renderer}.Routes(r)
	}
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(auth.NewSessionValidator(sessions), cfg.CookieSecure))
		r.Use(auth.CSRFProtect)
		dbmHandlers.APIRoutes(r)

		if legacyHTML {
			authHandlers.ProtectedRoutes(r)
			dbmHandlers.Routes(r)
			dbmHandlers.ApprovalRoutes(r)

			adminOpts := []admin.Option{
				admin.WithActivityLogger(activityLogger),
				admin.WithUserAdmin(adminUsers, sessions, renderer),
				admin.WithGroupAdmin(groupStore, adminUsers, connLister, sessions, renderer),
				admin.WithDashboard(adminUsers, groupStore, connLister, renderer),
			}
			if !devMode {
				adminOpts = append(adminOpts, admin.WithConnectionAdmin(connStore, renderer, registry, sessions))
			}
			admin.NewHandlers(adminOpts...).Routes(r)
		}
	})

	if !legacyHTML {
		spaDir := os.Getenv("SPA_DIST_DIR")
		if spaDir == "" {
			spaDir = "frontend/dist"
		}
		r.NotFound(web.SPAHandler(spaDir).ServeHTTP)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	var serveErr error
	select {
	case serveErr = <-errCh:
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if serveErr != nil {
		return serveErr
	}
	return shutdownErr
}
