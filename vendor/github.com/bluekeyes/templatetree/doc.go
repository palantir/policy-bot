// Package templatetree loads standard library templates in a way that creates
// a template hierarchy with inheritance. Templates may extend other templates
// by using a special comment as the first line of the template:
//
//     // in templates/base.tmpl
//     base := `Header
//     {{block "body" .}}Body{{end}}
//     Footer`
//
//     // in templates/a.tmpl
//     a := `{{/* templatetree:extends base.tmpl */}}
//     {{define "body"}}Body A{{end}}`
//
//     // in templates/b.tmpl
//     b := `{{/* templatetree:extends base.tmpl */}}
//     {{define "body"}}Body B{{end}}`
//
//     t, err := templatetree.LoadText("templates", "*.tmpl", nil)
//     // ... handle err
//
//     t.ExecuteTemplate(os.Stdout, "a.tmpl", nil)
//     // => Header
//     //    Body A
//     //    Footer
//
//     t.ExecuteTemplate(os.Stdout, "b.tmpl", nil)
//     // => Header
//     //    Body B
//     //    Footer
//
// Template trees may be arbitrarily deep and there may be multiple trees in a
// given directory.
//
// The comment marking extension must be the first line of the template and
// must exactly match the following, including whitespace:
//
//     {{/* templatetree:extends parent-template-name */}}
//
// Templates are named after their slash-separated file path relative to
// directory that was loaded.
//
// To define functions or set other options on the templates, pass a non-nil
// root template as the final argument to the Load* function.
package templatetree
