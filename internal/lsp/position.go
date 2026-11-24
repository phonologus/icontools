package lsp

import (
	"sort"
	"unicode/utf16"
	"unicode/utf8"
)

// mapper converts between byte offsets in a document and LSP positions,
// whose character counts are UTF-16 code units unless the client agreed to
// UTF-8 (byte) offsets.
type mapper struct {
	src    string
	starts []int // byte offset of each line start
	utf8   bool
}

func newMapper(src string, utf8 bool) *mapper {
	m := &mapper{src: src, starts: []int{0}, utf8: utf8}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			m.starts = append(m.starts, i+1)
		}
	}
	return m
}

// position returns the LSP position of byte offset off.
func (m *mapper) position(off int) Position {
	off = max(0, min(off, len(m.src)))
	line := sort.Search(len(m.starts), func(i int) bool { return m.starts[i] > off }) - 1
	return Position{Line: line, Character: m.units(m.src[m.starts[line]:off])}
}

func (m *mapper) rangeOf(start, end int) Range {
	return Range{m.position(start), m.position(end)}
}

// offset returns the byte offset of an LSP position, clamped to the line.
func (m *mapper) offset(p Position) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(m.starts) {
		return len(m.src)
	}
	start := m.starts[p.Line]
	end := len(m.src)
	if p.Line+1 < len(m.starts) {
		end = m.starts[p.Line+1] - 1 // the newline
	}
	n := 0
	for i, r := range m.src[start:end] {
		if n >= p.Character {
			return start + i
		}
		n += m.width(r)
	}
	return end
}

// units returns the length of s in position units.
func (m *mapper) units(s string) int {
	if m.utf8 {
		return len(s)
	}
	n := 0
	for _, r := range s {
		n += m.width(r)
	}
	return n
}

func (m *mapper) width(r rune) int {
	if m.utf8 {
		return utf8.RuneLen(r)
	}
	if utf16.RuneLen(r) == 2 {
		return 2
	}
	return 1
}
