package builtin

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Docs holds detailed documentation for built-ins, in Markdown, taken from
// the Icon website's reference pages. It is not distributed with icontools:
// FetchDocs downloads the pages to a local cache, and LoadDocs reads them.
type Docs struct {
	Functions map[string]string // by function name
	Keywords  map[string]string // by keyword name, without "&"
}

// refPages are the reference pages, relative to the Icon site.
var refPages = []string{
	"refernce/funcad.htm",
	"refernce/funcej.htm",
	"refernce/funcko.htm",
	"refernce/funcpr.htm",
	"refernce/funcsz.htm",
	"refernce/keywlist.htm",
	"docs/ipd281/gipd_a.htm", // graphics procedures
	"docs/ipd281/gipd_b.htm", // graphics keywords
}

// DefaultCacheDir returns the directory where FetchDocs stores the pages
// by default: iconls/ref in the user's cache directory.
func DefaultCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "iconls", "ref"), nil
}

// FetchDocs downloads the reference pages into dir, reporting progress to
// log.
func FetchDocs(dir string, log io.Writer) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	for _, p := range refPages {
		url := site + p
		fmt.Fprintln(log, "fetching", url)
		resp, err := client.Get(url)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: %s", url, resp.Status)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(p)), body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// LoadDocs reads whichever reference pages are present in dir. It returns
// nil if there are none.
func LoadDocs(dir string) *Docs {
	d := &Docs{Functions: map[string]string{}, Keywords: map[string]string{}}
	found := false
	for _, p := range refPages {
		b, err := os.ReadFile(filepath.Join(dir, filepath.Base(p)))
		if err != nil {
			continue
		}
		found = true
		if strings.HasSuffix(p, "keywlist.htm") {
			parseKeywordList(string(b), d.Keywords)
		} else {
			parseEntries(string(b), d)
		}
	}
	if !found {
		return nil
	}
	return d
}

var (
	anchorRE  = regexp.MustCompile(`(?i)<A\s+NAME\s*=\s*"([^"]+)"\s*>`)
	titleEnd  = regexp.MustCompile(`(?i)</H3>|<BR>`)
	hrRE      = regexp.MustCompile(`(?i)<HR>`)
	keywordRE = regexp.MustCompile(`^&(\w+)`)
	funcRE    = regexp.MustCompile(`^(\w+)\s*\(`)
)

// parseEntries parses a page of entries separated by <HR>, each a title
// line ("name(args) : result -- summary" or "&name : type -- summary") and
// a body. Function entries also carry a NAME anchor, which distinguishes
// them from introductory text.
func parseEntries(page string, d *Docs) {
	for _, chunk := range hrRE.Split(page, -1) {
		loc := titleEnd.FindStringIndex(chunk)
		if loc == nil {
			continue
		}
		title := text(chunk[:loc[0]])
		body := chunk[loc[1]:]
		if m := keywordRE.FindStringSubmatch(title); m != nil {
			d.Keywords[m[1]] = entry(title, body)
			continue
		}
		m := funcRE.FindStringSubmatch(title)
		if m == nil || !anchorRE.MatchString(chunk[:loc[0]]) {
			continue // not a function entry (e.g. the page's introduction)
		}
		d.Functions[m[1]] = entry(title, body)
	}
}

// entry renders a title and HTML body as Markdown.
func entry(title, body string) string {
	sig, summary, _ := strings.Cut(title, "--")
	sig = strings.Join(strings.Fields(sig), " ")
	sig = strings.Replace(sig, " (", "(", 1)
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**", sig)
	if s := strings.TrimSpace(summary); s != "" {
		fmt.Fprintf(&b, " — %s", s)
	}
	if md := markdown(body); md != "" {
		b.WriteString("\n\n")
		b.WriteString(md)
	}
	return b.String()
}

// parseKeywordList parses keywlist.htm: a <PRE> block of lines
// "&name : type  # summary", with continuation lines "  # more".
func parseKeywordList(page string, kws map[string]string) {
	var name, sig string
	var summary []string
	flush := func() {
		if name != "" {
			kws[name] = fmt.Sprintf("**%s** — %s", sig, strings.Join(summary, " "))
		}
		name = ""
	}
	for line := range strings.SplitSeq(html.UnescapeString(stripTags(page)), "\n") {
		t := strings.TrimSpace(line)
		code, comment, _ := strings.Cut(t, "#")
		code, comment = strings.TrimSpace(code), strings.TrimSpace(comment)
		switch {
		case strings.HasPrefix(code, "&"):
			flush()
			name = strings.TrimPrefix(strings.Fields(code)[0], "&")
			sig = strings.Join(strings.Fields(code), " ")
			summary = []string{comment}
		case code == "" && comment != "" && name != "":
			summary = append(summary, comment)
		}
	}
	flush()
}

var (
	tagRE  = regexp.MustCompile(`<[^>]*>`)
	preRE  = regexp.MustCompile(`(?is)<PRE>(.*?)</PRE>`)
	codeRE = regexp.MustCompile(`(?is)<CODE>(.*?)</CODE>`)
	brRE   = regexp.MustCompile(`(?i)<BR>|<P>`)
)

func stripTags(s string) string { return tagRE.ReplaceAllString(s, "") }

// text converts an HTML fragment to plain text on one line.
func text(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(stripTags(s))), " ")
}

// markdown converts an entry body to Markdown: paragraphs are filled,
// <CODE> becomes `code`, and <PRE> blocks become code blocks, except for
// "See also" lines, which become text.
func markdown(body string) string {
	var out []string
	para := func(s string) {
		s = codeRE.ReplaceAllString(s, "`$1`")
		for _, p := range brRE.Split(s, -1) {
			if t := text(p); t != "" {
				out = append(out, t)
			}
		}
	}
	rest := body
	for {
		loc := preRE.FindStringSubmatchIndex(rest)
		if loc == nil {
			para(rest)
			break
		}
		para(rest[:loc[0]])
		var code, also []string
		for l := range strings.SplitSeq(html.UnescapeString(stripTags(rest[loc[2]:loc[3]])), "\n") {
			switch t := strings.TrimRight(l, " \t\r"); {
			case strings.HasPrefix(strings.TrimSpace(t), "See also"):
				also = append(also, strings.TrimSpace(t))
			case strings.TrimSpace(t) != "" || len(code) > 0:
				code = append(code, t)
			}
		}
		for len(code) > 0 && strings.TrimSpace(code[len(code)-1]) == "" {
			code = code[:len(code)-1]
		}
		if len(code) > 0 {
			out = append(out, "```\n"+strings.Join(code, "\n")+"\n```")
		}
		out = append(out, also...)
		rest = rest[loc[1]:]
	}
	return strings.Join(out, "\n\n")
}
