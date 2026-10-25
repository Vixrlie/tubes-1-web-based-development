package web

import "html/template"

const connectionTestID = "connection-test"

type ConnectionTestResult struct {
	OK     bool
	Title  string
	Detail string
}

type ConnectionTestPanel struct {
	Action string
	Result *ConnectionTestResult
}

func (p ConnectionTestPanel) Button() Button {
	label := "Tes Koneksi"
	if p.Result != nil && !p.Result.OK {
		label = "Tes Ulang"
	}
	b := OutlineButton(label, "")
	b.Attrs = template.HTMLAttr(`hx-post="` + template.HTMLEscapeString(p.Action) +
		`" hx-target="#` + connectionTestID + `" hx-swap="outerHTML" hx-include="#connection-form" hx-disabled-elt="this"`)
	return b
}
