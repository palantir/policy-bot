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

// StdTemplate is a union of the standard library template types.
type StdTemplate interface {
	*text.Template | *html.Template
}

// Template contains the common functions of text/template.Template and
// html/template.Template used by this package.
type Template[T StdTemplate] interface {
	Name() string
	Execute(w io.Writer, data any) error
	ExecuteTemplate(w io.Writer, name string, data any) error
	Parse(text string) (T, error)
}

// Tree is a hierarchy of templates, mapping name to template. The concrete
// type of the values will be T.
type Tree[T StdTemplate] map[string]Template[T]

// ExecuteTemplate renders the template with the given name. See the
// text/template package for more details.
func (tree Tree[T]) ExecuteTemplate(wr io.Writer, name string, data any) error {
	if tmpl, ok := tree[name]; ok {
		return tmpl.Execute(wr, data)
	}
	return fmt.Errorf("templatetree: no template %q", name)
}

// TemplateFactory creates new empty templates.
type TemplateFactory[T StdTemplate] func(name string) Template[T]

// DefaultTextFactory uses text/template.New to create templates.
func DefaultTextFactory(name string) Template[*text.Template] {
	return text.New(name)
}

// DefaultHTMLFactory uses html/template.New to create templates.
func DefaultHTMLFactory(name string) Template[*html.Template] {
	return html.New(name)
}

// Parse recursively loads all templates in dir with names matching pattern,
// respecting inheritance. Templates are named by their paths relative to dir.
func Parse[T StdTemplate](dir, pattern string, f TemplateFactory[T]) (Tree[T], error) {
	return ParseFS(os.DirFS(dir), pattern, f)
}

// ParseFS recursively parses all templates in fsys with names matching
// pattern, respecting inheritance. Templates are named by their paths in fsys.
func ParseFS[T StdTemplate](fsys fs.FS, pattern string, f TemplateFactory[T]) (Tree[T], error) {
	files, err := loadFiles(fsys, pattern)
	if err != nil {
		return nil, err
	}
	return ParseFiles(files, f)
}

// ParseFiles parses all templates in files, respecting inheritance. Templates
// are named by their key in files, which maps name to content.
func ParseFiles[T StdTemplate](files map[string]string, f TemplateFactory[T]) (Tree[T], error) {
	if f == nil {
		return nil, fmt.Errorf("templatetree: factory must be non-nil")
	}

	nodes := make(map[string]*node[T])
	for name, f := range files {
		nodes[name] = &node[T]{name: name, content: f}
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
	tree := make(Tree[T])
	for {
		n := findNext(nodes)
		if n == nil {
			break
		}
		delete(nodes, n.name)

		t := f(n.name)
		if err := parseInto(t, n); err != nil {
			return nil, err
		}

		n.template = t
		tree[n.name] = t
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

type node[T StdTemplate] struct {
	name     string
	content  string
	template Template[T]
	parent   *node[T]
}

func parseInto[T StdTemplate](dst Template[T], src *node[T]) error {
	if src.parent != nil {
		// parents are already parsed, so we know they are valid
		_ = parseInto(dst, src.parent)
	}
	_, err := dst.Parse(src.content)
	return err
}

func findNext[T StdTemplate](nodes map[string]*node[T]) *node[T] {
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
