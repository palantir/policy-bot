package codeowners

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// ErrNoCodeownersFound is returned when no CODEOWNERS file is found
var ErrNoCodeownersFound = errors.New("no CODEOWNERS found")

// Codeowners - patterns/owners mappings for the given repo
type Codeowners struct {
	repoRoot string
	Patterns []Codeowner
}

// Codeowner - owners for a given pattern
type Codeowner struct {
	Pattern string
	re      *regexp.Regexp
	Owners  []string
}

func (c Codeowner) String() string {
	return fmt.Sprintf("%s\t%v", c.Pattern, strings.Join(c.Owners, ", "))
}

func dirExists(fsys fs.FS, path string) (bool, error) {
	fi, err := fs.Stat(fsys, path)
	if err == nil && fi.IsDir() {
		return true, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, err
}

// findCodeownersFile - find a CODEOWNERS file somewhere within or below
// the working directory (wd), and open it.
func findCodeownersFile(fsys fs.FS, wd string) (io.Reader, string, error) {
	dir := wd
	for {
		for _, p := range []string{".github", ".", "docs", ".gitlab"} {
			pth := path.Join(dir, p)
			exists, err := dirExists(fsys, pth)
			if err != nil {
				return nil, "", err
			}
			if exists {
				f := path.Join(pth, "CODEOWNERS")
				_, err := fs.Stat(fsys, f)
				if err != nil {
					if errors.Is(err, fs.ErrNotExist) {
						continue
					}
					return nil, "", err
				}
				r, err := fsys.Open(f)
				return r, dir, err
			}
		}
		odir := dir
		dir = path.Dir(odir)
		// if we can't go up any further...
		if odir == dir {
			break
		}
		// if we're heading above the volume name (relevant on Windows)...
		if len(dir) < len(filepath.VolumeName(odir)) {
			break
		}
	}
	return nil, "", nil
}

// Deprecated: Use [FromFile] instead.
func NewCodeowners(path string) (*Codeowners, error) {
	return FromFile(path)
}

// FromFile creates a Codeowners from the path to a local file. Consider using
// [FromFileWithFS] instead.
func FromFile(path string) (*Codeowners, error) {
	base := "/"
	if filepath.IsAbs(path) && filepath.VolumeName(path) != "" {
		base = path[:len(filepath.VolumeName(path))+1]
	}
	path = path[len(base):]

	return FromFileWithFS(os.DirFS(base), path)
}

// FromFileWithFS creates a Codeowners from the path to a file relative to the
// given filesystem.
func FromFileWithFS(fsys fs.FS, path string) (*Codeowners, error) {
	r, root, err := findCodeownersFile(fsys, path)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("%w in %s", ErrNoCodeownersFound, path)
	}
	return FromReader(r, root)
}

// FromReader creates a Codeowners from a given Reader instance and root path.
func FromReader(r io.Reader, repoRoot string) (*Codeowners, error) {
	co := &Codeowners{
		repoRoot: repoRoot,
	}
	patterns, err := parseCodeowners(r)
	if err != nil {
		return nil, err
	}
	co.Patterns = patterns
	return co, nil
}

func isSection(line string) bool {
	return strings.HasPrefix(line, "^") || strings.HasPrefix(line, "[")
}

// parseCodeowners parses a list of Codeowners from a Reader
func parseCodeowners(r io.Reader) ([]Codeowner, error) {
	co := []Codeowner{}
	s := bufio.NewScanner(r)
	var defaultOwners []string
	for s.Scan() {
		line := s.Text()
		if isSection(line) {
			defaultOwners = parseDefaultOwners(line)
			continue
		}
		fields := strings.Fields(line)

		fields = removeComments(fields)
		if len(fields) == 0 {
			continue
		}
		if len(fields) > 1 {
			fields = combineEscapedSpaces(fields)
			c, err := NewCodeowner(fields[0], fields[1:])
			if err != nil {
				return nil, err
			}
			co = append(co, c)
		} else if len(fields) == 1 && defaultOwners != nil {
			c, err := NewCodeowner(fields[0], defaultOwners)
			if err != nil {
				return nil, err
			}
			co = append(co, c)
		}
	}
	return co, nil
}

func removeComments(fields []string) []string {
	for i := range fields {
		if strings.HasPrefix(fields[i], "#") {
			fields = fields[:i]
			break
		}
	}
	return fields
}

func parseDefaultOwners(line string) []string {
	index := strings.LastIndex(line, "]")
	if index != -1 && len(line) > index+1 {
		return strings.Fields(strings.TrimSpace(line[index+1:]))
	}
	return nil
}

// if any of the elements ends with a \, it was an escaped space
// put it back together properly so it's not treated as separate fields
func combineEscapedSpaces(fields []string) []string {
	outFields := make([]string, 0)
	escape := `\`
	for i := 0; i < len(fields); i++ {
		outField := fields[i]
		for strings.HasSuffix(fields[i], escape) && i+1 < len(fields) {
			outField = strings.Join([]string{strings.TrimRight(outField, escape), fields[i+1]}, " ")
			i++
		}
		outFields = append(outFields, outField)
	}

	return outFields
}

// NewCodeowner -
func NewCodeowner(pattern string, owners []string) (Codeowner, error) {
	re, err := getPattern(pattern)
	if err != nil {
		return Codeowner{}, err
	}
	c := Codeowner{
		Pattern: pattern,
		re:      re,
		Owners:  owners,
	}
	return c, nil
}

// Owners - return the list of code owners for the given path
// (within the repo root)
func (c *Codeowners) Owners(path string) []string {
	if i := c.PatternIndex(path); i >= 0 {
		return c.Patterns[i].Owners
	}
	return nil
}

// PatternIndex - return the index of the pattern that matches the given path
// (within the repo root)
func (c *Codeowners) PatternIndex(path string) int {
	if strings.HasPrefix(path, c.repoRoot) {
		path = strings.Replace(path, c.repoRoot, "", 1)
	}

	// Order is important; the last matching pattern takes the most precedence.
	for i := len(c.Patterns) - 1; i >= 0; i-- {
		if c.Patterns[i].re.MatchString(path) {
			return i
		}
	}

	return -1
}

// precompile all regular expressions
var (
	rePrependSlash = regexp.MustCompile(`([^\/+])/.*\*\.`)
)

// based on github.com/sabhiram/go-gitignore
// but modified so that 'dir/*' only matches files in 'dir/'
func getPattern(line string) (*regexp.Regexp, error) {
	// when # or ! is escaped with a \
	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	}

	// If we encounter a foo/*.blah in a folder, prepend the / char
	if rePrependSlash.MatchString(line) && line[0] != '/' {
		line = "/" + line
	}

	// Handle escaping the "." char
	line = strings.ReplaceAll(line, ".", `\.`)

	magicStar := "#$~"

	// Handle "/**/" usage
	if strings.HasPrefix(line, "/**/") {
		line = line[1:]
	}
	line = strings.ReplaceAll(line, `/**/`, `(/|/.+/)`)
	line = strings.ReplaceAll(line, `**/`, `(|.`+magicStar+`/)`)
	line = strings.ReplaceAll(line, `/**`, `(|/.`+magicStar+`)`)

	// Handle escaping the "*" char
	line = strings.ReplaceAll(line, `\*`, `\`+magicStar)
	line = strings.ReplaceAll(line, `*`, `([^/]*)`)

	// Handle escaping the "?" char
	line = strings.ReplaceAll(line, "?", `\?`)

	line = strings.ReplaceAll(line, magicStar, "*")

	// Temporary regex
	expr := ""

	switch {
	case strings.HasSuffix(line, "/"):
		expr = line + "(|.*)$"
	case strings.HasSuffix(line, "/([^/]*)"):
		expr = line + "$"
	default:
		expr = line + "($|/.*$)"
	}

	if strings.HasPrefix(expr, "/") {
		expr = "^(|/)" + expr[1:]
	} else {
		expr = "^(|.*/)" + expr
	}

	return regexp.Compile(expr)
}
