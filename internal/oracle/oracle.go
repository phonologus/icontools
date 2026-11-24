// Package oracle runs icont's own front end, via tools/icontok, as ground
// truth for tests. Tests using it are skipped unless $ICONTOK names the
// icontok binary (see tools/icontok/build.sh).
package oracle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phonologus/icontools/internal/corpus"
	"github.com/phonologus/icontools/internal/lexer"
	"github.com/phonologus/icontools/internal/token"
)

// Oracle wraps the icontok and icont binaries.
type Oracle struct {
	Root       string // reference Icon tree
	icontok    string
	icont      string
	predefined map[string]bool
}

// New returns an Oracle, or skips the test if one is not available.
func New(t testing.TB) *Oracle {
	t.Helper()
	tool := os.Getenv("ICONTOK")
	if tool == "" {
		t.Skip("ICONTOK not set; see tools/icontok/build.sh")
	}
	o := &Oracle{
		Root:    corpus.Require(t),
		icontok: tool,
		// icont itself, built alongside icontok by build.sh.
		icont: filepath.Join(filepath.Dir(tool), "..", "..", "bin", "icont"),
	}
	src, err := os.ReadFile(filepath.Join(o.Root, "src", "h", "features.h"))
	if err != nil {
		t.Fatal(err)
	}
	o.predefined = map[string]bool{}
	for line := range strings.SplitSeq(string(src), "\n") {
		if _, rest, ok := strings.Cut(line, "Feature("); ok {
			if f := strings.Split(rest, `"`); len(f) > 1 {
				o.predefined[f[1]] = true
			}
		}
	}
	return o
}

// Source is a corpus file prepared for comparison with icont.
type Source struct {
	File     string // original file
	Text     string // text to analyse: the file, or its preprocessed form
	Path     string // file holding Text, for passing to icontok
	Expanded bool   // Text is icont -E output
	LineDirs bool   // Text contains #line comments, so icont's line numbers differ
}

// Prepare reads file. If it needs the preprocessor (directives, or use of
// the symbols ipp predefines), Text is the output of "icont -E" with the
// IPL include directories on the search path, written to a temporary file.
// ok is false if icont cannot preprocess the file.
func (o *Oracle) Prepare(t testing.TB, file string) (s Source, ok bool) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	s = Source{File: file, Text: string(b), Path: file}
	if o.needsPreprocessing(s.Text) {
		cmd := exec.Command(o.icont, "-E", filepath.Base(file))
		cmd.Dir = filepath.Dir(file)
		cmd.Env = append(os.Environ(), "LPATH="+
			filepath.Join(o.Root, "ipl", "incl")+" "+filepath.Join(o.Root, "ipl", "gincl"))
		out, err := cmd.Output()
		if err != nil {
			return s, false
		}
		s.Text, s.Expanded = string(out), true
		s.Path = filepath.Join(t.TempDir(), filepath.Base(file))
		if err := os.WriteFile(s.Path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.LineDirs = hasLineComment(s.Text)
	return s, true
}

func (o *Oracle) needsPreprocessing(src string) bool {
	toks, _ := lexer.Tokenize(src)
	for _, t := range toks {
		if t.Kind == token.IDENT && o.predefined[t.Text] {
			return true
		}
		for _, tr := range t.Leading {
			if tr.Kind == lexer.Directive {
				return true
			}
		}
	}
	return false
}

func hasLineComment(src string) bool {
	toks, _ := lexer.Tokenize(src)
	for _, t := range toks {
		for _, tr := range t.Leading {
			if tr.Kind == lexer.Comment && strings.HasPrefix(tr.Text, "#line") {
				return true
			}
		}
	}
	return false
}

// Tokens runs icont's lexer on s. ok is false if icont reports errors.
func (o *Oracle) Tokens(s Source) (out []byte, ok bool) {
	out, _, ok = o.run(s, "")
	return out, ok
}

// Trees runs icont's parser on s, returning one S-expression line per
// procedure, and icont's error messages. ok is false if icont reports
// errors.
func (o *Oracle) Trees(s Source) (out []byte, msgs []string, ok bool) {
	return o.run(s, "-t")
}

func (o *Oracle) run(s Source, flag string) ([]byte, []string, bool) {
	args := []string{s.Path}
	if flag != "" {
		args = []string{flag, s.Path}
	}
	cmd := exec.Command(o.icontok, args...)
	cmd.Dir = filepath.Dir(s.File)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	var msgs []string
	for line := range strings.SplitSeq(stderr.String(), "\n") {
		if line != "" {
			msgs = append(msgs, line)
		}
	}
	return out, msgs, err == nil
}

// Icont runs icont with args in directory dir, with the IPL include
// directories on the search path, and returns its diagnostic output.
// ok is false if icont exits with an error.
func (o *Oracle) Icont(dir string, args ...string) (stderr string, ok bool) {
	cmd := exec.Command(o.icont, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LPATH="+
		filepath.Join(o.Root, "ipl", "incl")+" "+filepath.Join(o.Root, "ipl", "gincl"))
	var b strings.Builder
	cmd.Stderr = &b
	cmd.Stdout = &b
	err := cmd.Run()
	return b.String(), err == nil
}
