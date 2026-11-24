// Command icfmt formats Icon source code in the style of the Icon Program
// Library.
//
// Usage:
//
//	icfmt [-l] [-w] [-d] [path ...]
//
// With no paths it reads standard input and writes standard output. Paths
// may be files or directories (searched recursively for .icn files).
//
// Like gofmt, icfmt keeps the programmer's line breaks and normalises
// indentation and spacing. It never changes the token sequence that icont
// sees, and it refuses to format files with syntax errors.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/phonologus/icontools/internal/format"
)

var (
	list  = flag.Bool("l", false, "list files whose formatting differs from icfmt's")
	write = flag.Bool("w", false, "write result to (source) file instead of stdout")
	diff  = flag.Bool("d", false, "display diffs instead of rewriting files")
)

var exitCode = 0

func report(err error) {
	fmt.Fprintln(os.Stderr, err)
	exitCode = 2
}

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: icfmt [-l] [-w] [-d] [path ...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() == 0 {
		if *write {
			fmt.Fprintln(os.Stderr, "icfmt: cannot use -w with standard input")
			os.Exit(2)
		}
		if err := process("<standard input>", os.Stdin, os.Stdout); err != nil {
			report(err)
		}
		os.Exit(exitCode)
	}
	for _, path := range flag.Args() {
		st, err := os.Stat(path)
		if err != nil {
			report(err)
			continue
		}
		if !st.IsDir() {
			processFile(path)
			continue
		}
		filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				report(err)
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(p, ".icn") {
				processFile(p)
			}
			return nil
		})
	}
	os.Exit(exitCode)
}

func processFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		report(err)
		return
	}
	defer f.Close()
	if err := process(path, f, os.Stdout); err != nil {
		report(err)
	}
}

func process(name string, in io.Reader, out io.Writer) error {
	src, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	res, err := format.Source(src)
	if err != nil {
		return fmt.Errorf("%s:\n%v", name, indent(err.Error()))
	}
	changed := !bytes.Equal(src, res)
	if *list && changed {
		fmt.Fprintln(out, name)
	}
	if *write && changed {
		st, err := os.Stat(name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(name, res, st.Mode().Perm()); err != nil {
			return err
		}
	}
	if *diff && changed {
		d, err := unifiedDiff(name, src, res)
		if err != nil {
			return err
		}
		out.Write(d)
	}
	if !*list && !*write && !*diff {
		out.Write(res)
	}
	return nil
}

func indent(s string) string {
	return "\t" + strings.ReplaceAll(s, "\n", "\n\t")
}

// unifiedDiff runs diff -u on the two versions.
func unifiedDiff(name string, a, b []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "icfmt")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	fa, fb := filepath.Join(dir, "orig"), filepath.Join(dir, "new")
	if err := os.WriteFile(fa, a, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(fb, b, 0o600); err != nil {
		return nil, err
	}
	out, err := exec.Command("diff", "-u", "--label", name+".orig", "--label", name, fa, fb).Output()
	if len(out) > 0 {
		return out, nil // diff exits 1 when the files differ
	}
	return nil, err
}
