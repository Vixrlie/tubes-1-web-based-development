package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/admin"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/config"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/db"
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
	r := web.NewRouter()

	// Rute publik: belum ada session (login punya perlindungan Origin sendiri).
	authHandlers.PublicRoutes(r)

	// Halaman uji komponen (khusus development, tanpa login).
	if os.Getenv("DEV_COMPONENTS") == "1" {
		web.DevRoutes{R: renderer}.Routes(r)
	}

	// Rute terlindungi: session wajib valid, ...
	r.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(auth.NewSessionValidator(sessions), cfg.CookieSecure))
		r.Use(auth.CSRFProtect)

		authHandlers.ProtectedRoutes(r)
		admin.NewHandlers(
			admin.WithUserAdmin(adminUsers, sessions, renderer),
		).Routes(r)
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
