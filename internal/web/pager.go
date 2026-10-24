package web

import (
	"fmt"
	"sort"
)

// Tipe data untuk komponen paginasi (templates/components/pager.html).
// Bisa dipakai tabel apa pun; tabel memakainya lewat Table.Pager.
//
//	tbl.Pager = web.NewPager("Menampilkan 1–50 dari 1.250 baris", page, 25,
//	    func(p int) string { return fmt.Sprintf("?tab=data&page=%d", p) })
//
// Di template: {{template "pager" .Pager}}

// PagerItem = satu tombol. Gap = "..." (tidak bisa diklik).
type PagerItem struct {
	Label    string
	Href     string
	Active   bool // halaman saat ini
	Disabled bool // mis. "Prev" di halaman pertama
	Gap      bool
}

type Pager struct {
	Label   string // aria-label navigasi (default "Paginasi")
	Summary string // teks kiri, mis. "Menampilkan 1–4 dari 1.250 baris"
	Items   []PagerItem
}

// NewPager membuat « Prev, nomor halaman (halaman pertama, terakhir, dan
// tetangga halaman aktif; sisanya diringkas "..."), Next ».
// href dipanggil dengan nomor halaman (mulai 1). Kurang dari 2 halaman -> nil
// (komponen tidak dirender).
func NewPager(summary string, page, totalPages int, href func(page int) string) *Pager {
	if totalPages < 2 {
		return nil
	}
	page = min(max(page, 1), totalPages)

	set := map[int]bool{1: true, totalPages: true, page - 1: true, page: true, page + 1: true}
	nums := make([]int, 0, len(set))
	for n := range set {
		if n >= 1 && n <= totalPages {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)

	items := []PagerItem{{Label: "« Prev", Href: href(page - 1), Disabled: page == 1}}
	prev := 0
	for _, n := range nums {
		switch {
		case prev != 0 && n-prev == 2: // jarak 1 halaman: tampilkan saja, bukan "..."
			items = append(items, PagerItem{Label: fmt.Sprint(prev + 1), Href: href(prev + 1)})
		case prev != 0 && n-prev > 2:
			items = append(items, PagerItem{Gap: true, Label: "..."})
		}
		items = append(items, PagerItem{Label: fmt.Sprint(n), Href: href(n), Active: n == page})
		prev = n
	}
	items = append(items, PagerItem{Label: "Next »", Href: href(page + 1), Disabled: page == totalPages})
	return &Pager{Summary: summary, Items: items}
}
