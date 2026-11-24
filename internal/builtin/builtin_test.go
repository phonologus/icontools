package builtin

import (
	"strings"
	"testing"
)

func TestCatalog(t *testing.T) {
	if n := len(Functions()); n < 150 {
		t.Errorf("only %d functions", n)
	}
	f, ok := LookupFunction("left")
	if !ok || f.Signature != "left(s1,i,s2)" || f.Arity != 3 || f.Graphics || f.Summary == "" {
		t.Errorf("left: %+v", f)
	}
	if got := strings.Join(f.Params(), " "); got != "s1 i s2" {
		t.Errorf("left params: %s", got)
	}
	if f.URL() != "https://www2.cs.arizona.edu/icon/refernce/funcko.htm#left" {
		t.Errorf("left URL: %s", f.URL())
	}
	g, ok := LookupFunction("DrawLine")
	if !ok || !g.Graphics || g.Arity != -1 {
		t.Errorf("DrawLine: %+v", g)
	}
	for _, f := range Functions() {
		if f.Summary == "" {
			t.Errorf("%s has no summary", f.Name)
		}
	}
	for _, name := range []string{"subject", "pos", "fail", "null", "window"} {
		if k, ok := LookupKeyword(name); !ok || k.Summary == "" {
			t.Errorf("&%s: %+v", name, k)
		}
	}
	if _, ok := LookupFunction("nosuch"); ok {
		t.Error("found nosuch")
	}
}

func TestDocs(t *testing.T) {
	d := LoadDocs("testdata/ref")
	if d == nil {
		t.Fatal("no docs")
	}
	want := map[string]string{
		"left": "**left(s1,i,s2) : s3** — put string on the left\n\n" +
			"Pads s1 on the right with copies of s2 to make a string of length i.\n\n" +
			"```\nDefaults:\ni       1\ns2      \" \"\n```\n\n" +
			"See also: right()",
		"Clone": "**Clone(W, s1,..., sn) : W** — make a context\n\n" +
			"`Clone()` makes a context for `W`.\n\nSee also: Couple()",
	}
	for name, w := range want {
		if got := d.Functions[name]; got != w {
			t.Errorf("%s:\n%s\nwant:\n%s", name, got, w)
		}
	}
	if len(d.Functions) != 2 {
		t.Errorf("functions: %v", d.Functions)
	}
	wantK := map[string]string{
		"col":       "**&col : i** — mouse column\n\nThe mouse column at the last event.",
		"allocated": "**&allocated : i1,i2** — bytes allocated (total,static)",
		"fail":      "**&fail** — fails",
	}
	for name, w := range wantK {
		if got := d.Keywords[name]; got != w {
			t.Errorf("&%s:\n%q\nwant:\n%q", name, got, w)
		}
	}
	if LoadDocs(t.TempDir()) != nil {
		t.Error("docs from empty dir")
	}
}
