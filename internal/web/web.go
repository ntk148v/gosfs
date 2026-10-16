// Package web holds the embedded HTML template rendered for directory listings.
package web

import (
	"embed"
	"html/template"
	"io"
)

//go:embed index.html
var files embed.FS

var tmpl = template.Must(template.New("index").Parse(mustRead("index.html")))

func mustRead(name string) string {
	b, err := files.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Render executes the directory-listing template into w.
func Render(w io.Writer, data any) error { return tmpl.Execute(w, data) }
