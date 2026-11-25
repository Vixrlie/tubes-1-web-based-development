package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadRequiresPlatformDBURL(t *testing.T) {
	t.Setenv("PLATFORM_DB_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("PLATFORM_DB_URL kosong diterima")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("PLATFORM_DB_URL", "postgres://x/y")
	t.Setenv("APP_ADDR", "")
	t.Setenv("COOKIE_SECURE", "")
	t.Setenv("SESSION_IDLE_TIMEOUT", "")
	t.Setenv("SESSION_ABSOLUTE_TIMEOUT", "")
	t.Setenv("SESSION_ROTATE_INTERVAL", "")
	t.Setenv("SESSION_GRACE_PERIOD", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.CookieSecure {
		t.Error("CookieSecure default harus false")
	}
	if cfg.SessionIdle != 30*time.Minute || cfg.SessionAbsolute != 8*time.Hour ||
		cfg.SessionRotate != 15*time.Minute || cfg.SessionGrace != 30*time.Second {
		t.Errorf("timeout default = %v/%v/%v/%v", cfg.SessionIdle, cfg.SessionAbsolute, cfg.SessionRotate, cfg.SessionGrace)
	}
}

func TestLoadRejectsBadDuration(t *testing.T) {
	t.Setenv("PLATFORM_DB_URL", "postgres://x/y")
	t.Setenv("SESSION_IDLE_TIMEOUT", "bukan-durasi")
	if _, err := Load(); err == nil {
		t.Fatal("durasi invalid diterima")
	}
}

func TestLoadDotEnvDoesNotOverrideEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "DARI_FILE=ya\nDARI_ENV=harus-menang\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("tulis .env: %v", err)
	}

	t.Setenv("DARI_ENV", "harus-menang")
	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DARI_FILE"); got != "ya" {
		t.Errorf("DARI_FILE = %q, want ya", got)
	}
	if got := os.Getenv("DARI_ENV"); got != "harus-menang" {
		t.Errorf("env asli ditimpa .env: %q", got)
	}
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "tidak-ada.env")); err != nil {
		t.Errorf("file hilang = error, want nil (bukan fatal): %v", err)
	}
}
