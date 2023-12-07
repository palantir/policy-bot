// Package templatetree loads standard library templates in a way that creates
// a template hierarchy with inheritance. Templates extend other templates by
// placing a special comment as the first line of the template. Template trees
// may be arbitrarily deep and multiple independent tree can be loaded at the
// same time.
//
// The comment marking extension must be the first line of the template and
// must exactly match the following, including whitespace:
//
//	{{/* templatetree:extends parent-template-name */}}
//
// If loading from a file system, a template's name is its slash-separated file
// path relative to the root. If parsing a map of files, the key sets the
// template's name.
//
// To define functions or set other options on the templates, provide a custom
// factory function.
package templatetree
