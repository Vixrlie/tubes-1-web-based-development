package web

import (
	"context"
	"log"
	"strings"

	"github.com/Vixrlie/tubes-1-web-based-development/internal/auth"
	"github.com/Vixrlie/tubes-1-web-based-development/internal/session"
)

// DatabaseLink adalah database yang boleh dibuka user. Level berasal dari
// akses efektif tertinggi dari seluruh group yang dianggotai user.
type DatabaseLink struct {
	ID          int64
	Name        string
	Description string
	HostPort    string
	Level       string // "view" atau "edit"
}

type AppShell struct {
	Title       string
	Me          auth.User
	CSRFToken   string
	Databases   []DatabaseLink
	Breadcrumbs []Crumb
	CurrentDB   int64
	ActiveNav   string
	Notice      string
	NoticeDesc  string
	Initials    string

	// Halaman admin baru ditampilkan di sidebar bila halamannya benar-benar ada.
	CanManageUsers       bool
	CanManageConnections bool
}

type ShellStore interface {
	ListForUser(ctx context.Context, userID int64) ([]DatabaseLink, error)
}

type ShellOptions struct {
	Title                string
	CurrentDB            int64
	ActiveNav            string
	CanManageUsers       bool
	CanManageConnections bool
}

// BuildShell mengumpulkan data bersama app shell dari context request.
// Kegagalan membaca daftar database tidak menghentikan halaman: sidebar lalu
// tampil tanpa daftar.
func BuildShell(ctx context.Context, store ShellStore, opts ShellOptions) AppShell {
	me, _ := auth.UserFromContext(ctx)
	shell := AppShell{
		Title:                opts.Title,
		Me:                   me,
		CSRFToken:            session.CSRFToken(ctx),
		CurrentDB:            opts.CurrentDB,
		ActiveNav:            opts.ActiveNav,
		Initials:             Initials(me.Username),
		CanManageUsers:       opts.CanManageUsers,
		CanManageConnections: opts.CanManageConnections,
	}
	if store != nil {
		links, err := store.ListForUser(ctx, me.ID)
		if err != nil {
			log.Printf("web shell: daftar database: %v", err)
		} else {
			shell.Databases = links
		}
	}
	return shell
}

func Initials(username string) string {
	var out strings.Builder
	for _, part := range strings.Split(username, ".") {
		if part == "" {
			continue
		}
		out.WriteString(strings.ToUpper(part[:1]))
		if out.Len() == 2 {
			break
		}
	}
	return out.String()
}
