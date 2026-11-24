// Package corpus locates the reference Icon source tree (a clone of
// https://github.com/gtownsend/icon) used as ground truth and test corpus.
//
// The location is $ICON_SRC if set, otherwise ../icon relative to the
// repository root.
package corpus

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Root returns the reference tree's directory, or "" if it cannot be found.
func Root() string {
	if d := os.Getenv("ICON_SRC"); d != "" {
		return d
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	// file = <repo>/internal/corpus/corpus.go
	d := filepath.Join(filepath.Dir(file), "..", "..", "..", "icon")
	if st, err := os.Stat(filepath.Join(d, "src", "common", "lextab.h")); err == nil && !st.IsDir() {
		return filepath.Clean(d)
	}
	return ""
}

// Require returns Root() or skips the test if the tree is absent.
func Require(t testing.TB) string {
	t.Helper()
	r := Root()
	if r == "" {
		t.Skip("reference Icon tree not found; set ICON_SRC")
	}
	return r
}

// IconFiles returns every .icn file under the reference tree, sorted.
func IconFiles(t testing.TB) []string {
	t.Helper()
	root := Require(t)
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".icn") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
