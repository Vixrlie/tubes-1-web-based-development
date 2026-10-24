package web

type Shell struct {
	Nav  string     // "database" | "admin" -> menandai link aktif
	User *ShellUser // nil = belum login
}

type ShellUser struct {
	Username        string
	FullName        string
	AvatarURL       string // kosong -> tampil inisial
	IsPlatformAdmin bool
}

func (u *ShellUser) DisplayName() string {
	if u.FullName != "" {
		return u.FullName
	}
	return u.Username
}

func (u *ShellUser) Initial() string {
	for _, r := range u.DisplayName() {
		return string(r)
	}
	return "?"
}
