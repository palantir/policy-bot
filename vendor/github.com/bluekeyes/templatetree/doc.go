// Package templatetree loads standard library templates in a way that creates
// a template hierarchy with inheritance. Templates extend other templates by
// placing a special comment as the first line of the template. Template trees
// may be arbitrarily deep and multiple independent tree can be loaded at the
// same time.
//
// The comment marking extension must be the first line of the template and
// must exactly match the following, including whitespace:
//
//     {{/* templatetree:extends parent-template-name */}}
//
// If loading from a directory, a template's name is its slash-separated file
// path relative to the directory. If parsing a slice of templatetree.File
// structs, the Name field in the struct sets the template's name.
//
// To define functions or set other options on the templates, provide a non-nil
// factory function.
package templatetree
