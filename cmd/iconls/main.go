// Command iconls is a Language Server Protocol server for Icon.
//
// It speaks LSP over standard input and output, as editors expect:
//
//	iconls [-log file] [-docs dir]
//
// Diagnostics about the server itself go to standard error, or to the
// file named by -log.
//
// Hover documentation for built-in functions and keywords comes from the
// Icon sources (a one-line summary for each). For fuller documentation,
// download the reference pages from the Icon website to a local cache:
//
//	iconls fetch-docs [-dir dir]
//
// The cache is iconls/ref in the user's cache directory unless -dir (or
// -docs, when serving) names another. The pages are not part of icontools.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/phonologus/icontools/internal/builtin"
	"github.com/phonologus/icontools/internal/lsp"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "fetch-docs" {
		fetchDocs(os.Args[2:])
		return
	}
	logFile := flag.String("log", "", "write server log to `file` (default: standard error)")
	docsDir := flag.String("docs", "", "read reference pages from `dir` (default: the fetch-docs cache)")
	version := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *version {
		fmt.Println("iconls", lsp.Version)
		return
	}
	var logw io.Writer = os.Stderr
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "iconls:", err)
			os.Exit(2)
		}
		defer f.Close()
		logw = f
	}
	dir := *docsDir
	if dir == "" {
		dir, _ = builtin.DefaultCacheDir()
	}
	err := lsp.Run(os.Stdin, os.Stdout, logw, lsp.Options{Docs: builtin.LoadDocs(dir)})
	switch {
	case errors.Is(err, lsp.ErrExitWithoutShutdown):
		os.Exit(1)
	case err != nil:
		fmt.Fprintln(logw, "iconls:", err)
		os.Exit(1)
	}
}

func fetchDocs(args []string) {
	fs := flag.NewFlagSet("fetch-docs", flag.ExitOnError)
	def, err := builtin.DefaultCacheDir()
	if err != nil {
		def = ""
	}
	dir := fs.String("dir", def, "cache `directory`")
	fs.Parse(args)
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "iconls: no cache directory; use -dir")
		os.Exit(2)
	}
	if err := builtin.FetchDocs(*dir, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "iconls:", err)
		os.Exit(1)
	}
	d := builtin.LoadDocs(*dir)
	fmt.Fprintf(os.Stderr, "iconls: %d functions and %d keywords documented in %s\n",
		len(d.Functions), len(d.Keywords), *dir)
}
