// Command iconls is a Language Server Protocol server for Icon.
//
// It speaks LSP over standard input and output, as editors expect:
//
//	iconls [-log file]
//
// Diagnostics about the server itself go to standard error, or to the
// file named by -log.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/phonologus/icontools/internal/lsp"
)

func main() {
	logFile := flag.String("log", "", "write server log to `file` (default: standard error)")
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
	err := lsp.Run(os.Stdin, os.Stdout, logw)
	switch {
	case errors.Is(err, lsp.ErrExitWithoutShutdown):
		os.Exit(1)
	case err != nil:
		fmt.Fprintln(logw, "iconls:", err)
		os.Exit(1)
	}
}
