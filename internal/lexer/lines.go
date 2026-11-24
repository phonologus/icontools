package lexer

import "sort"

// Lines maps byte offsets in a source text to line numbers.
type Lines struct {
	starts []int // byte offset at which each line begins
}

// NewLines indexes the line starts of src.
func NewLines(src string) *Lines {
	ls := &Lines{starts: []int{0}}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			ls.starts = append(ls.starts, i+1)
		}
	}
	return ls
}

// Position returns the 1-based line and 0-based byte column of off.
func (ls *Lines) Position(off int) (line, col int) {
	i := sort.Search(len(ls.starts), func(i int) bool { return ls.starts[i] > off }) - 1
	return i + 1, off - ls.starts[i]
}
