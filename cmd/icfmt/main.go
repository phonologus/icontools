// Command icfmt formats Icon source code.
//
// Usage:
//
//	icfmt [-l] [-w] [-d] [path ...]
//
// With no paths it reads standard input and writes standard output.
package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	list  = flag.Bool("l", false, "list files whose formatting differs from icfmt's")
	write = flag.Bool("w", false, "write result to (source) file instead of stdout")
	diff  = flag.Bool("d", false, "display diffs instead of rewriting files")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: icfmt [-l] [-w] [-d] [path ...]")
		flag.PrintDefaults()
	}
	flag.Parse()
	_, _, _ = *list, *write, *diff
	fmt.Fprintln(os.Stderr, "icfmt: not implemented yet")
	os.Exit(2)
}
