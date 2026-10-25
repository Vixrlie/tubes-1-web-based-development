package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr          string
	Env           string
	PlatformDBURL string
	CookieSecure  bool

	AdminUsername string
	AdminPassword string

	SessionIdle     time.Duration
	SessionAbsolute time.Duration
	SessionRotate   time.Duration
	SessionGrace    time.Duration
}

func Load() (Config, error) {
	c := Config{
		Addr:          get("APP_ADDR", ":8080"),
		Env:           get("APP_ENV", "development"),
		PlatformDBURL: os.Getenv("PLATFORM_DB_URL"),
		CookieSecure:  get("COOKIE_SECURE", "false") == "true",
		AdminUsername: os.Getenv("ADMIN_USERNAME"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}
	if c.PlatformDBURL == "" {
		return c, errors.New("PLATFORM_DB_URL wajib diisi")
	}

	var err error
	if c.SessionIdle, err = dur("SESSION_IDLE_TIMEOUT", "30m"); err != nil {
		return c, err
	}
	if c.SessionAbsolute, err = dur("SESSION_ABSOLUTE_TIMEOUT", "8h"); err != nil {
		return c, err
	}
	if c.SessionRotate, err = dur("SESSION_ROTATE_INTERVAL", "15m"); err != nil {
		return c, err
	}
	if c.SessionGrace, err = dur("SESSION_GRACE_PERIOD", "30s"); err != nil {
		return c, err
	}
	if strings.EqualFold(strings.TrimSpace(c.Env), "production") && !c.CookieSecure {
		return c, errors.New("APP_ENV=production mewajibkan COOKIE_SECURE=true")
	}
	return c, nil
}

func get(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func dur(key, def string) (time.Duration, error) {
	return time.ParseDuration(get(key, def))
}
