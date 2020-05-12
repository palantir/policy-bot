# templatetree [![GoDoc](https://godoc.org/github.com/bluekeyes/templatetree?status.svg)](http://godoc.org/github.com/bluekeyes/templatetree)

templatetree is a standard library template loader that creates simple template
inheritance trees. Base templates use the `block` or `template` directives to
define sections that are overridden or provided by child templates.

Functions are provided to create both `text/template` and `html/template`
objects.

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


Use `LoadHTML` to load and render the templates:

```go
package main

import (
	"html/template"
	"os"
	"time"

	"github.com/bluekeyes/templatetree"
)

func main() {
	root := template.New("root").Funcs(template.FuncMap{
		"now": func() string {
			return time.Now().Format(time.RFC3339)
		},
	})

	t, err := templatetree.LoadHTML("templates", "*.html.tmpl", root)
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


## Stability

While the API is simple, it hasn't seen heavy use yet and may change in the
future. I recommend vendoring this package at a specific commit if you are
concerned about API changes.
