package web

// TabItem = satu item di komponen tabs (templates/components/tabs.html).
//
//	tabs := []web.TabItem{
//	    {Label: "Column", Href: "?tab=columns", Active: true},
//	    {Label: "Data", Href: "?tab=data"},
//	}
type TabItem struct {
	Label  string
	Href   string
	Active bool
}
