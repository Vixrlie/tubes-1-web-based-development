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
	"github.com/Vixrlie/tubes-1-web-based-development/internal/admin"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/async"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/config"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/db"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/dbm"
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

	adminUsers := admin.NewPGUserStore(pool)

	// Akses database per group + registry pool target untuk route /databases/*.
	accessPolicy := access.NewPolicy(access.NewPGLevelSource(pool))
	registry := dbm.NewURLRegistry(pool)
	defer registry.Close()
	connStore := admin.NewPGConnectionStore(pool)
	groupStore := admin.NewPGGroupStore(pool)
	connLister := admin.NewPGConnectionLister(pool)
	migrationStore := dbm.NewPGMigrationStore(pool)
	asyncStore := async.NewPGStore(pool)
	asyncLocks := async.NewPGLockManager(pool)
	asyncHistory := async.NewPGHistoryRecorder(pool)
	asyncExecutor := async.NewTargetExecutor(asyncStore, registry, nil)
	asyncWorker := async.NewWorker(async.WorkerConfig{
		Store:    asyncStore,
		Locks:    asyncLocks,
		Executor: asyncExecutor,
		History:  asyncHistory,
	})

	if *workerOnly {
		asyncWorker.Start(ctx)
		return nil
	}

	asyncHandlers := async.NewHandlers(async.HandlerConfig{
		Store:      asyncStore,
		Migrations: migrationStore,
		Policy:     accessPolicy,
	})
	dbmHandlers := dbm.NewHandlers(dbm.Config{
		Platform:   pool,
		Policy:     accessPolicy,
		Registry:   registry,
		Renderer:   renderer,
		Migrations: migrationStore,
	})

	// Mode developing
	devMode := os.Getenv("DEV_COMPONENTS") == "1"
	r := web.NewRouter()

	authHandlers.PublicRoutes(r)

	if devMode {
		web.DevRoutes{R: renderer}.Routes(r)
	}

	// Rute protected
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(auth.NewSessionValidator(sessions), cfg.CookieSecure))
		r.Use(auth.CSRFProtect)

		authHandlers.ProtectedRoutes(r)
		dbmHandlers.Routes(r)
		asyncHandlers.Routes(r)

		adminOpts := []admin.Option{
			admin.WithUserAdmin(adminUsers, sessions, renderer),
			admin.WithGroupAdmin(groupStore, adminUsers, connLister, sessions, renderer),
			admin.WithDashboard(adminUsers, groupStore, connLister, renderer),
		}
		if !devMode {
			adminOpts = append(adminOpts,
				admin.WithConnectionAdmin(connStore, renderer, registry, sessions))
		}
		admin.NewHandlers(adminOpts...).Routes(r)
	})

	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		asyncWorker.Start(workerCtx)
	}()

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

	stopWorker()
	<-workerDone

	if serveErr != nil {
		return serveErr
	}
	return shutdownErr
}
