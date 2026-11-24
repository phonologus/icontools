package format

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// align adjusts horizontal space at the alignment points recorded while
// printing, in the manner of gofmt:
//
//   - In a run of consecutive lines holding single-line case clauses of the
//     same case expression, the clause bodies start in a common column.
//   - In a run of consecutive lines with trailing comments, the comments
//     start in a common column, one space beyond the longest code.
//
// Lines are counted from 0 as in printer.line.
func (p *printer) align(out string) string {
	if len(p.marks) == 0 {
		return out
	}
	lines := strings.Split(out, "\n")
	var ls []int
	for l := range p.marks {
		ls = append(ls, l)
	}
	sort.Ints(ls)

	// Case clause bodies.
	runs(ls, func(l int) (int, bool) {
		m := p.marks[l]
		return m.caseGroup, m.caseAt >= 0
	}, func(run []int) {
		w := 0
		for _, l := range run {
			w = max(w, width(lines[l][:p.marks[l].caseAt]))
		}
		for _, l := range run {
			m := p.marks[l]
			old := lines[l]
			lines[l] = pad(old, m.caseAt, w)
			if m.commentAt > m.caseAt {
				m.commentAt += len(lines[l]) - len(old)
			}
		}
	})

	// Trailing comments.
	runs(ls, func(l int) (int, bool) {
		return 0, p.marks[l].commentAt >= 0
	}, func(run []int) {
		w := 0
		for _, l := range run {
			w = max(w, width(lines[l][:p.marks[l].commentAt]))
		}
		for _, l := range run {
			lines[l] = pad(lines[l], p.marks[l].commentAt, w)
		}
	})
	return strings.Join(lines, "\n")
}

// runs calls f for each maximal run of consecutive line numbers in ls that
// have a mark (per has) with equal group.
func runs(ls []int, has func(int) (group int, ok bool), f func([]int)) {
	var run []int
	group := 0
	flush := func() {
		if len(run) > 0 {
			f(run)
		}
		run = nil
	}
	for _, l := range ls {
		g, ok := has(l)
		if !ok {
			flush()
			continue
		}
		if len(run) > 0 && (l != run[len(run)-1]+1 || g != group) {
			flush()
		}
		run = append(run, l)
		group = g
	}
	flush()
}

// pad replaces the blanks at offset at in line so that the text after
// them starts at column w+1.
func pad(line string, at, w int) string {
	rest := strings.TrimLeft(line[at:], " \t")
	return line[:at] + strings.Repeat(" ", w-width(line[:at])+1) + rest
}

// width returns the display width of s, counting each rune (or invalid
// byte) as one column and tabs to the next multiple of 8.
func width(s string) int {
	n := 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if r == '\t' {
			n = (n/8 + 1) * 8
		} else {
			n++
		}
		s = s[size:]
	}
	return n
}
