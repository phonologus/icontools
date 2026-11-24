package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/phonologus/icontools/internal/lexer"
	"github.com/phonologus/icontools/internal/sema"
	"github.com/phonologus/icontools/internal/syntax"
)

// workspace finds and parses the files of Icon programs. Open documents
// take precedence over the disk; files read from disk are cached by
// modification time and size.
type workspace struct {
	open  map[string]*document // by path
	cache map[string]*diskFile // by path
	ipath []string             // extra directories to search for links
	utf8  bool
}

type diskFile struct {
	mod  time.Time
	size int64
	doc  *document
}

func newWorkspace() *workspace {
	return &workspace{open: map[string]*document{}, cache: map[string]*diskFile{}}
}

// uriToPath converts a file: URI to a path ("" if it is not one).
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	return filepath.Clean(u.Path)
}

func pathToURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// load returns the document for path: the open one, or the file on disk.
func (w *workspace) load(path string) *document {
	if d := w.open[path]; d != nil {
		return d
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return nil
	}
	if c := w.cache[path]; c != nil && c.mod.Equal(st.ModTime()) && c.size == st.Size() {
		return c.doc
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	d := newDocument(pathToURI(path), 0, string(b), w.utf8)
	w.cache[path] = &diskFile{mod: st.ModTime(), size: st.Size(), doc: d}
	return d
}

func newDocument(uri string, version int, text string, utf8 bool) *document {
	d := &document{uri: uri, path: uriToPath(uri), version: version, text: text}
	d.file = syntax.Parse(text)
	d.m = newMapper(text, utf8)
	d.sema = &sema.File{Path: d.path, Syntax: d.file}
	if d.path == "" {
		d.sema.Path = uri
	}
	return d
}

// definesMain reports whether d declares procedure main.
func definesMain(d *document) bool {
	for _, decl := range d.file.Decls {
		if p, ok := decl.(*syntax.ProcDecl); ok && p.Name != nil && p.Name.Text == "main" {
			return true
		}
	}
	return false
}

// program assembles the program containing d: d itself; the other .icn
// files in its directory, except those defining main when d does (they
// are other programs); and, transitively, the files they link.
func (w *workspace) program(d *document) (*sema.Program, map[*sema.File]*document) {
	docs := map[string]*document{}
	var order []*document
	add := func(x *document) bool {
		key := x.path
		if key == "" {
			key = x.uri
		}
		if docs[key] != nil {
			return false
		}
		docs[key] = x
		order = append(order, x)
		return true
	}
	add(d)
	if d.path != "" {
		dir := filepath.Dir(d.path)
		mainHere := definesMain(d)
		var paths []string
		if ents, err := os.ReadDir(dir); err == nil {
			for _, e := range ents {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".icn") {
					paths = append(paths, filepath.Join(dir, e.Name()))
				}
			}
		}
		for p := range w.open {
			if filepath.Dir(p) == dir && strings.HasSuffix(p, ".icn") {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		for _, p := range paths {
			if x := w.load(p); x != nil && !(mainHere && x != d && definesMain(x)) {
				add(x)
			}
		}
	}
	complete := true
	for i := 0; i < len(order); i++ {
		for _, name := range links(order[i]) {
			x, found := w.findLink(name, order[i])
			switch {
			case x != nil:
				add(x)
			case !found:
				complete = false
			default:
				complete = false // present only as ucode: names unknown
			}
		}
	}
	files := make([]*sema.File, len(order))
	byFile := map[*sema.File]*document{}
	for i, x := range order {
		x.sema.Macros = w.macros(x, map[string]bool{x.path: true})
		files[i] = x.sema
		byFile[x.sema] = x
	}
	return sema.NewProgram(files, complete), byFile
}

// links returns the names in d's link declarations.
func links(d *document) []string {
	var names []string
	for _, decl := range d.file.Decls {
		if l, ok := decl.(*syntax.LinkDecl); ok {
			for _, t := range l.Files.Elems {
				if t != nil {
					names = append(names, strings.Trim(t.Text, `"`))
				}
			}
		}
	}
	return names
}

// searchDirs returns the directories searched for a file linked from
// from: its own directory, the configured ipath, then $IPATH and $LPATH.
func (w *workspace) searchDirs(from *document) []string {
	var dirs []string
	if from.path != "" {
		dirs = append(dirs, filepath.Dir(from.path))
	}
	dirs = append(dirs, w.ipath...)
	for _, v := range []string{"IPATH", "LPATH"} {
		dirs = append(dirs, strings.Fields(os.Getenv(v))...)
	}
	return dirs
}

// findLink locates the source of a linked file. It returns the document
// if the source is found; otherwise found reports whether the file exists
// as ucode (name.u1), whose declarations we cannot see.
func (w *workspace) findLink(name string, from *document) (d *document, found bool) {
	name = strings.TrimSuffix(name, ".icn")
	for _, dir := range w.searchDirs(from) {
		base := name
		if !filepath.IsAbs(base) {
			base = filepath.Join(dir, name)
		}
		if x := w.load(base + ".icn"); x != nil {
			return x, true
		}
		if _, err := os.Stat(base + ".u1"); err == nil {
			found = true
		}
	}
	return nil, found
}

// linkTarget returns the path of the source file a link name refers to.
func (w *workspace) linkTarget(name string, from *document) string {
	if x, _ := w.findLink(name, from); x != nil {
		return x.path
	}
	return ""
}

// macros computes the preprocessor symbols in force in each part of d,
// following $define, $undef and $include directives in order. Included
// files are searched for in d's directory, the configured ipath, and
// $LPATH. stack guards against include cycles.
func (w *workspace) macros(d *document, stack map[string]bool) []sema.MacroScope {
	const end = int(^uint(0) >> 1)
	var out []sema.MacroScope
	active := map[string]int{} // name -> index in out of the definition in force
	define := func(def *sema.MacroDef, from int) {
		if i, ok := active[def.Name]; ok {
			out[i].Until = from
		}
		active[def.Name] = len(out)
		out = append(out, sema.MacroScope{Def: def, From: from, Until: end})
	}
	for _, t := range d.file.Tokens {
		for _, tr := range t.Leading {
			if tr.Kind != lexer.Directive {
				continue
			}
			cmd, name, nameOff, rest := parseDirective(tr.Text)
			switch cmd {
			case "define":
				if name == "" {
					continue
				}
				off := tr.Offset + nameOff
				define(&sema.MacroDef{Name: name, Value: rest, Path: d.path, Offset: off, End: off + len(name)}, tr.Offset)
			case "undef":
				if i, ok := active[name]; ok {
					out[i].Until = tr.Offset
					delete(active, name)
				}
			case "include":
				file := strings.Trim(strings.TrimSpace(name+rest), `"`)
				inc := w.findInclude(file, d)
				if inc == nil || stack[inc.path] {
					continue
				}
				stack[inc.path] = true
				for _, m := range w.macros(inc, stack) {
					if m.Until == end { // still defined at the end of the included file
						define(m.Def, tr.Offset)
					}
				}
				delete(stack, inc.path)
			}
		}
	}
	return out
}

// parseDirective splits a directive line "$cmd name rest # comment" into
// its command, first word (with its offset in line) and the rest, with
// any comment removed.
func parseDirective(line string) (cmd, name string, nameOff int, rest string) {
	i := strings.IndexByte(line, '$') + 1
	skip := func() {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
	}
	word := func() string {
		j := i
		for i < len(line) && (isWordByte(line[i])) {
			i++
		}
		return line[j:i]
	}
	skip()
	cmd = word()
	skip()
	nameOff = i
	name = word()
	if name == "" && i < len(line) && line[i] == '"' {
		return cmd, "", nameOff, stripComment(line[i:])
	}
	return cmd, name, nameOff, strings.TrimSpace(stripComment(line[i:]))
}

func isWordByte(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c|0x20 && c|0x20 <= 'z'
}

// stripComment removes a trailing # comment, ignoring # inside quotes.
func stripComment(s string) string {
	var q byte
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case q != 0 && c == '\\':
			i++
		case q != 0 && c == q:
			q = 0
		case q == 0 && (c == '"' || c == '\''):
			q = c
		case q == 0 && c == '#':
			return strings.TrimSpace(s[:i])
		}
	}
	return strings.TrimSpace(s)
}

// findInclude locates a $include file: relative to the including file,
// then in the configured ipath and $LPATH directories.
func (w *workspace) findInclude(name string, from *document) *document {
	if filepath.IsAbs(name) {
		return w.load(name)
	}
	var dirs []string
	if from.path != "" {
		dirs = append(dirs, filepath.Dir(from.path))
	}
	dirs = append(dirs, w.ipath...)
	dirs = append(dirs, strings.Fields(os.Getenv("LPATH"))...)
	for _, dir := range dirs {
		if d := w.load(filepath.Join(dir, name)); d != nil {
			return d
		}
	}
	return nil
}
