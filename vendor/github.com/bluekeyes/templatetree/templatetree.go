package templatetree

import (
	"fmt"
	html "html/template"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	text "text/template"
)

const (
	// CommentTagExtends is the tag used in template comments to mark a
	// template's parent.
	CommentTagExtends = "templatetree:extends"

	// DefaultRootTemplateName is the name of the root template when none is
	// provided by the caller.
	DefaultRootTemplateName = "[templatetree:root]"
)

// LoadText recursively loads all text templates in dir with names matching
// pattern, respecting inheritance. If the root template is nil, a new default
// template is used.
//
// Templates are named as normalized paths relative to dir.
func LoadText(dir, pattern string, root *text.Template) (TextTree, error) {
	if root == nil {
		root = text.New(DefaultRootTemplateName)
	}

	tree := make(TextTree)
	return tree, loadAll(dir, pattern, textTemplate{root}, func(name string, t template) {
		tree[name] = t.(textTemplate).Template
	})
}

// LoadHTML recursively loads all HTML templates in dir with names matching
// pattern, respecting inheritance. If the root template is nil, a new default
// template is used.
//
// Templates are named as normalized paths relative to dir.
func LoadHTML(dir, pattern string, root *html.Template) (HTMLTree, error) {
	if root == nil {
		root = html.New(DefaultRootTemplateName)
	}

	tree := make(HTMLTree)
	return tree, loadAll(dir, pattern, htmlTemplate{root}, func(name string, t template) {
		tree[name] = t.(htmlTemplate).Template
	})
}

// TextTree is a hierarchy of text templates, mapping name to template.
type TextTree map[string]*text.Template

// ExecuteTemplate renders the template with the given name. See the
// text/template package for more details.
func (tree TextTree) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	if tmpl, ok := tree[name]; ok {
		return tmpl.Execute(wr, data)
	}
	return fmt.Errorf("templatetree: no template %q", name)
}

// HTMLTree is a hierarchy of text templates, mapping name to template.
type HTMLTree map[string]*html.Template

// ExecuteTemplate renders the template with the given name. See the
// html/template package for more details.
func (tree HTMLTree) ExecuteTemplate(wr io.Writer, name string, data interface{}) error {
	if tmpl, ok := tree[name]; ok {
		return tmpl.Execute(wr, data)
	}
	return fmt.Errorf("templatetree: no template %q", name)
}

// adapter for text/template and html/template
type template interface {
	Name() string
	Clone() (template, error)
	Parse(string) error
}

type textTemplate struct {
	*text.Template
}

func (t textTemplate) Clone() (template, error) {
	nt, err := t.Template.Clone()
	return textTemplate{nt}, err
}

func (t textTemplate) Parse(content string) error {
	_, err := t.Template.Parse(content)
	return err
}

type htmlTemplate struct {
	*html.Template
}

func (t htmlTemplate) Clone() (template, error) {
	nt, err := t.Template.Clone()
	return htmlTemplate{nt}, err
}

func (t htmlTemplate) Parse(content string) error {
	_, err := t.Template.Parse(content)
	return err
}

type node struct {
	name    string
	path    string
	content string

	template template
	parent   *node
}

func loadAll(dir, pattern string, root template, register func(string, template)) error {
	nodes := make(map[string]*node)
	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		match, err := filepath.Match(pattern, filepath.Base(path))
		if err != nil {
			return err
		}
		if match {
			name := filepath.ToSlash(strings.TrimPrefix(path, dir))
			name = strings.TrimPrefix(name, "/")
			nodes[name] = &node{name: name, path: path}
		}
		return nil
	}

	// find all the templates
	if err := filepath.Walk(dir, walkFn); err != nil {
		return err
	}

	// load all content and create links
	for _, n := range nodes {
		b, err := ioutil.ReadFile(n.path)
		if err != nil {
			return err
		}

		n.content = string(b)

		parent := parseHeader(n.content)
		if parent != "" {
			if p, ok := nodes[parent]; ok {
				n.parent = p
			} else {
				return fmt.Errorf("templatetree: template %q extends unknown template %s", n.name, parent)
			}
		}
	}

	// parse templates from the root nodes in/down
	for {
		n := findNext(nodes)
		if n == nil {
			break
		}
		delete(nodes, n.name)

		base := root
		if n.parent != nil {
			base = n.parent.template
		}

		t, err := base.Clone()
		if err != nil {
			return err
		}
		if err := t.Parse(n.content); err != nil {
			return formatParseError(n, t, err)
		}

		n.template = t
		register(n.name, t)
	}

	// check for cycles
	if len(nodes) > 0 {
		var names []string
		for _, n := range nodes {
			names = append(names, strconv.Quote(n.name))
		}
		return fmt.Errorf("templatetree: inheritance cycle in templates [%s]", strings.Join(names, ", "))
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

// The current template API doesn't provide a way to change names, so try to
// edit the error message so the correct name appears for users. This is dirty,
// but is strictly for usability, not correctness.
func formatParseError(n *node, t template, err error) error {
	msg := err.Error()
	old := "template: " + t.Name()
	if strings.HasPrefix(msg, old) {
		return fmt.Errorf("template: %s%s", n.name, strings.TrimPrefix(msg, old))
	}
	return err
}
