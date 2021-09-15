package templatetree

import (
	html "html/template"
	"io"
	text "text/template"
	"text/template/parse"
)

// Template defines common methods implemented by both *text/template.Template
// and *html/template.Template.
type Template interface {
	Name() string
	Execute(w io.Writer, data interface{}) error
	ExecuteTemplate(w io.Writer, name string, data interface{}) error
}

// template is an adapter interface for stdlib template types
type template interface {
	Unwrap() Template

	Name() string
	Tree() *parse.Tree
	AddParseTree(name string, tree *parse.Tree) error
	Templates() []template
	Parse(text string) error
}

type textTemplate struct {
	*text.Template
}

func (t textTemplate) Unwrap() Template  { return t.Template }
func (t textTemplate) Tree() *parse.Tree { return t.Template.Tree }

func (t textTemplate) AddParseTree(name string, tree *parse.Tree) error {
	_, err := t.Template.AddParseTree(name, tree)
	return err
}

func (t textTemplate) Templates() []template {
	ts := t.Template.Templates()
	tmpls := make([]template, len(ts))
	for i, tmpl := range ts {
		tmpls[i] = textTemplate{tmpl}
	}
	return tmpls
}

func (t textTemplate) Parse(text string) error {
	_, err := t.Template.Parse(text)
	return err
}

type htmlTemplate struct {
	*html.Template
}

func (t htmlTemplate) Unwrap() Template  { return t.Template }
func (t htmlTemplate) Tree() *parse.Tree { return t.Template.Tree }

func (t htmlTemplate) AddParseTree(name string, tree *parse.Tree) error {
	_, err := t.Template.AddParseTree(name, tree)
	if name == t.Name() {
		// html/template (as of 1.16.6) has an issue where it does not set the
		// Tree of the top-level template when the added tree replaces it.
		//
		// TODO(bkeyes): report this upstream and see if it's actually a bug,
		// since AddParseTree is not _really_ meant for public use.
		t.Template.Tree = tree
	}
	return err
}

func (t htmlTemplate) Templates() []template {
	ts := t.Template.Templates()
	tmpls := make([]template, len(ts))
	for i, tmpl := range ts {
		tmpls[i] = htmlTemplate{tmpl}
	}
	return tmpls
}

func (t htmlTemplate) Parse(text string) error {
	_, err := t.Template.Parse(text)
	return err
}
