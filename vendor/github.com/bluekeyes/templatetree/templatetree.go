package templatetree

import (
	"fmt"
	html "html/template"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	text "text/template"
)

const (
	// CommentTagExtends is the tag used in template comments to mark a
	// template's parent.
	CommentTagExtends = "templatetree:extends"
)

// Tree is a hierarchy of templates, mapping name to template. The concrete
// type of the values is determined by the TemplateFactory used when parsing
// and will be either *text/template.Template or *html/template.Template.
type Tree map[string]Template

// ExecuteTemplate renders the template with the given name. See the
// text/template package for more details.
func (tree Tree) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	if tmpl, ok := tree[name]; ok {
		return tmpl.Execute(wr, data)
	}
	return fmt.Errorf("templatetree: no template %q", name)
}

// TemplateFactory creates new empty templates.
type TemplateFactory interface {
	newTemplate(name string) template
}

// TextFactory is a TemplateFactory that creates text templates. If nil, it
// uses text/template.New to create templates.
type TextFactory func(name string) *text.Template

func (f TextFactory) newTemplate(name string) template {
	if f == nil {
		return textTemplate{text.New(name)}
	}
	return textTemplate{f(name)}
}

// HTMLFactory is a TemplateFactory that creates HTML templates. If nil, it
// uses html/template.New to create templates.
type HTMLFactory func(name string) *html.Template

func (f HTMLFactory) newTemplate(name string) template {
	if f == nil {
		return htmlTemplate{html.New(name)}
	}
	return htmlTemplate{f(name)}
}

// Parse recursively loads all templates in dir with names matching pattern,
// respecting inheritance. Templates are named by their paths relative to dir.
func Parse(dir, pattern string, f TemplateFactory) (Tree, error) {
	return ParseFS(os.DirFS(dir), pattern, f)
}

// ParseFS recursively parses all templates in fsys with names matching
// pattern, respecting inheritance. Templates are named by their paths in fsys.
func ParseFS(fsys fs.FS, pattern string, f TemplateFactory) (Tree, error) {
	files, err := loadFiles(fsys, pattern)
	if err != nil {
		return nil, err
	}
	return ParseFiles(files, f)
}

// ParseFiles parses all templates in files, respecting inheritance. Templates
// are named by their key in files, which maps name to content.
func ParseFiles(files map[string]string, f TemplateFactory) (Tree, error) {
	if f == nil {
		return nil, fmt.Errorf("templatetree: factory must be non-nil")
	}

	nodes := make(map[string]*node)
	for name, f := range files {
		nodes[name] = &node{name: name, content: f}
	}

	// create links between parents and children
	for _, n := range nodes {
		parent := parseHeader(n.content)
		if parent != "" {
			if p, ok := nodes[parent]; ok {
				n.parent = p
			} else {
				return nil, fmt.Errorf("templatetree: template %q extends unknown template %s", n.name, parent)
			}
		}
	}

	// parse templates from the root nodes in/down
	tree := make(Tree)
	for {
		n := findNext(nodes)
		if n == nil {
			break
		}
		delete(nodes, n.name)

		t := f.newTemplate(n.name)
		if n.parent != nil {
			if err := copyTemplates(t, n.parent.template); err != nil {
				return nil, err
			}
		}

		if err := t.Parse(n.content); err != nil {
			return nil, err
		}

		n.template = t
		tree[n.name] = t.Unwrap()
	}

	// check for cycles
	if len(nodes) > 0 {
		var names []string
		for _, n := range nodes {
			names = append(names, strconv.Quote(n.name))
		}
		return nil, fmt.Errorf("templatetree: inheritance cycle in templates [%s]", strings.Join(names, ", "))
	}

	return tree, nil
}

type node struct {
	name     string
	content  string
	template template
	parent   *node
}

func copyTemplates(dst, src template) error {
	for _, t := range src.Templates() {
		name := t.Name()
		if name == src.Name() {
			name = dst.Name() // copy top-level template in src to top-level in dst
		}
		if err := dst.AddParseTree(name, t.Tree()); err != nil {
			return err
		}
	}
	return nil
}

func findNext(nodes map[string]*node) *node {
	for _, n := range nodes {
		if n.parent == nil || n.parent.template != nil {
			return n
		}
	}
	return nil
}

func parseHeader(content string) (parent string) {
	prefix := "{{/* " + CommentTagExtends + " "
	if !strings.HasPrefix(content, prefix) {
		return
	}

	idx := strings.Index(content[len(prefix):], " */}}")
	if idx < 0 {
		return
	}

	parent = content[len(prefix) : len(prefix)+idx]
	return
}

func loadFiles(fsys fs.FS, pattern string) (map[string]string, error) {
	files := make(map[string]string)
	walkFn := func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		match, err := path.Match(pattern, path.Base(name))
		if err != nil {
			return err
		}
		if match {
			b, err := fs.ReadFile(fsys, name)
			if err != nil {
				return err
			}
			files[name] = string(b)
		}
		return nil
	}

	if err := fs.WalkDir(fsys, ".", walkFn); err != nil {
		return nil, err
	}
	return files, nil
}
