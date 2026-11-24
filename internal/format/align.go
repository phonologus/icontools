package format

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// align adjusts horizontal space at the alignment points recorded while
// printing, in the manner of gofmt:
//
//   - In a run of consecutive lines holding single-line case clauses of the
//     same case expression, the clause bodies start in a common column.
//   - In a run of consecutive lines at the same indentation, each holding
//     a single-line assignment statement, the assignment operators are
//     aligned on their ":" (so ":=" and "+:=" line up on ":="), if the
//     author aligned them.
//   - In a run of consecutive lines at the same indentation that end with
//     the same binary operator (as in a chain "a ||" / "b ||" / ...), the
//     operators start in a common column, if the author aligned them (any
//     of them was preceded by padding in the source).
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

	// Assignment statements.
	runs(ls, func(l int) (int, bool) {
		m := p.marks[l]
		return m.assignIndent, m.assignAt >= 0
	}, func(run []int) {
		// As for line-final operators, align only runs the author aligned.
		deliberate := false
		for _, l := range run {
			deliberate = deliberate || p.marks[l].assignPadded
		}
		if len(run) < 2 || !deliberate {
			return
		}
		// Align the anchor of each operator: its ":" (so that "+:=" and
		// ":=" line up on ":="), or its first character for "<-", "<->".
		col := 0
		for _, l := range run {
			m := p.marks[l]
			col = max(col, width(lines[l][:m.assignAt])+1+m.assignAnchor)
		}
		for _, l := range run {
			m := p.marks[l]
			old := lines[l]
			lines[l] = padTo(old, m.assignAt, col-m.assignAnchor)
			d := len(lines[l]) - len(old)
			if m.opAt > m.assignAt {
				m.opAt += d
			}
			if m.commentAt > m.assignAt {
				m.commentAt += d
			}
		}
	})

	// Line-final binary operators.
	groups := map[string]int{}
	runs(ls, func(l int) (int, bool) {
		m := p.marks[l]
		if m.opAt < 0 || !endsWithOp(lines[l], m) {
			return 0, false
		}
		key := fmt.Sprint(m.opIndent, " ", m.opText)
		if _, ok := groups[key]; !ok {
			groups[key] = len(groups) + 1
		}
		return groups[key], true
	}, func(run []int) {
		// Align only runs the author aligned: at least two lines, at
		// least one of which had padding before its operator.
		deliberate := false
		for _, l := range run {
			deliberate = deliberate || p.marks[l].opPadded
		}
		if len(run) < 2 || !deliberate {
			return
		}
		w := 0
		for _, l := range run {
			w = max(w, width(lines[l][:p.marks[l].opAt]))
		}
		for _, l := range run {
			m := p.marks[l]
			old := lines[l]
			lines[l] = pad(old, m.opAt, w)
			if m.commentAt > m.opAt {
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

// endsWithOp reports whether the operator recorded in m is the last code
// on the line (it may not be, if a following "{" was joined to the line).
func endsWithOp(line string, m *marks) bool {
	end := len(line)
	if m.commentAt >= 0 {
		end = m.commentAt
	}
	return m.opAt <= end && strings.TrimSpace(line[m.opAt:end]) == m.opText
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

// padTo replaces the blanks at offset at in line so that the text after
// them starts at column col (0-based).
func padTo(line string, at, col int) string {
	rest := strings.TrimLeft(line[at:], " \t")
	return line[:at] + strings.Repeat(" ", col-width(line[:at])) + rest
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
