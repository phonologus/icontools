package format

import (
	"strings"

	"github.com/phonologus/icontools/internal/lexer"
)

// VIB, the Icon Program Library's visual interface builder, maintains a
// section of the program between two marker comments (see
// ipl/gpacks/vib/vibfile.icn), rewriting it whenever the interface is
// saved. icfmt leaves such sections exactly as they are.
var vibMarkers = [][2]string{
	{"#===<<vib:begin>>===", "#===<<vib:end>>==="},
	{"#===<<xie:begin>>===", "#===<<xie:end>>==="}, // older files
}

// preserveVIB replaces the VIB sections of the formatted text out with the
// corresponding lines of src, markers included.
func preserveVIB(src, out string) string {
	ss, os := vibSections(src), vibSections(out)
	if len(ss) == 0 || len(ss) != len(os) { // (cannot happen: comments are kept)
		return out
	}
	srcLines := strings.Split(src, "\n")
	outLines := strings.Split(out, "\n")
	var b []string
	prev := 0
	for i, o := range os {
		s := ss[i]
		b = append(b, outLines[prev:o[0]]...)
		b = append(b, srcLines[s[0]:s[1]+1]...)
		prev = o[1] + 1
	}
	b = append(b, outLines[prev:]...)
	return strings.Join(b, "\n")
}

// vibSections returns the line ranges [begin, end] (0-based, inclusive) of
// the VIB sections of src: from a begin marker comment to the matching end
// marker, each on a line of its own.
func vibSections(src string) [][2]int {
	toks, _ := lexer.Tokenize(src)
	ls := lexer.NewLines(src)
	var secs [][2]int
	begin, kind := -1, -1
	for i, t := range toks {
		// Trivia follows the previous token, so it is at the start of a
		// line only at the start of the file or after a newline.
		atLineStart := i == 0
		for _, tr := range t.Leading {
			switch tr.Kind {
			case lexer.Newline:
				atLineStart = true
				continue
			case lexer.Space:
				continue
			case lexer.Comment:
				if atLineStart {
					line, _ := ls.Position(tr.Offset)
					for k, m := range vibMarkers {
						switch {
						case begin < 0 && strings.HasPrefix(tr.Text, m[0]):
							begin, kind = line-1, k
						case begin >= 0 && k == kind && strings.HasPrefix(tr.Text, m[1]):
							secs = append(secs, [2]int{begin, line - 1})
							begin = -1
						}
					}
				}
			}
			atLineStart = false
		}
	}
	return secs
}
