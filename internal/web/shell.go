package web

type Shell struct {
	Title      string     // judul tab (opsional; kosong = "Pathgres")
	Nav        string     // "database" | "admin" -> mode header + pilihan sidebar
	SideActive string     // item aktif sidebar admin: "users" | "groups" | "connections"
	User       *ShellUser // nil = belum login
	Sidebar    *SidebarDB // diisi saat Nav == "database"
}

// SidebarDB = isi sidebar mode database. Href & Active dihitung oleh handler.
type SidebarDB struct {
	Name             string // nama database (ditampilkan huruf besar lewat CSS)
	Schemas          []SidebarSchema
	MigrationsHref   string
	MigrationsActive bool
}

type SidebarSchema struct {
	Name   string
	Href   string
	Active bool
	Tables []SidebarTable
}

type SidebarTable struct {
	Name   string
	Href   string
	Active bool
}

type ShellUser struct {
	Username        string
	FullName        string
	AvatarURL       string // kosong -> tampil inisial
	IsPlatformAdmin bool
	// CSRFToken = token csrf sesi; dipakai form Logout di kartu akun (header).
	// Diisi handler yang membangun ShellUser, sama seperti token di data halaman.
	CSRFToken string
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
