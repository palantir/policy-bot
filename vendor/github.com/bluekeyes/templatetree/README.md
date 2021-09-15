# templatetree [![Go Reference](https://pkg.go.dev/badge/github.com/bluekeyes/templatetree.svg)](https://pkg.go.dev/github.com/bluekeyes/templatetree)

templatetree is a standard library template loader that creates simple template
inheritance trees. Base templates use the `block` or `template` directives to
define sections that are overridden or provided by child templates.

Compatible with both `text/template` and `html/template`.

## Example

Given a `templates` directory with the following content:

**templates/page.html.tmpl**

    <html>
      <head>
        <title>{{block "title" .}}{{end}}</title>
      </head>
      <body>
      {{block "body" .}}{{end}}
      </body>
    </html>

**templates/index.html.tmpl**

    {{/* templatetree:extends page.html.tmpl */}}
    {{define "title"}}{{.Animal}} Status{{end}}
    {{define "body"}}
      <p>The time is {{now}}</p>
      <p>The {{.Animal}} is {{.Status}}</p>
    {{end}}


Use `Parse` to load and render the templates:

```go
package main

import (
	"html/template"
	"os"
	"time"

	"github.com/bluekeyes/templatetree"
)

func main() {
	factory := templatetree.HTMLFactory(func(name string) *template.Template {
		return template.New(name).Funcs(template.FuncMap{
			"now": func() string {
				return time.Now().Format(time.RFC3339)
			},
		})
	})

	t, err := templatetree.Parse("templates", "*.html.tmpl", factory)
	if err != nil {
		panic(err)
	}

	var data struct {
		Animal string
		Status string
	}
	data.Animal = "Walrus"
	data.Status = "Forlorn"

	if err := t.ExecuteTemplate(os.Stdout, "index.html.tmpl", &data); err != nil {
		panic(err)
	}
}
```

Output:

    <html>
      <head>
        <title>Walrus Status</title>
      </head>
      <body>
        <p>The time is 2018-07-14T21:45:21.230-07:00</p>
        <p>The Walrus is Forlorn</p>
      </body>
    </html>

You can also load templates from a `fs.FS` using `templatetree.ParseFS` or
from memory using `templatetree.ParseFiles`. See the package documentation
for details and an example.

## Stability

The API was redesigned in v0.4.0 based on experience with previous versions
and should be more stable as a result. That said, I still consider this beta
software, with the possibility for more changes.
