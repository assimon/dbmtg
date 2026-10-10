// Package web 内嵌前端模板和静态资源，编译进二进制。
package web

import (
	"embed"
	"html/template"
)

//go:embed templates
var fs embed.FS

var Templates = template.Must(template.ParseFS(fs, "templates/*.html"))
