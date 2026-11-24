// Package builtin describes Icon's built-in functions and keywords.
//
// The catalogue (names, arities, signatures and one-line summaries) is
// generated from the public-domain Icon sources by tools/mkbuiltin; see
// catalog_gen.go. Richer documentation from the Icon website can be
// loaded from a local cache; see Docs.
package builtin

import "strings"

//go:generate go run ../../tools/mkbuiltin -o catalog_gen.go

// Function is a built-in function.
type Function struct {
	Name      string
	Arity     int    // number of parameters, or -1 if variadic
	Signature string // e.g. "left(s1,i,s2)"
	Summary   string // one line, from the Icon runtime sources
	Graphics  bool   // part of the graphics facilities
}

// Predefined is a symbol the Icon preprocessor defines (as 1) on systems
// with the corresponding feature, e.g. _UNIX or _GRAPHICS.
type Predefined struct {
	Name    string
	Feature string // the &features entry, or "" if unconditional
}

// LookupPredefined returns the predefined preprocessor symbol name.
func LookupPredefined(name string) (*Predefined, bool) {
	for i := range predefined {
		if predefined[i].Name == name {
			return &predefined[i], true
		}
	}
	return nil, false
}

// Keyword is a built-in keyword (without its "&").
type Keyword struct {
	Name    string
	Summary string
}

var (
	funcIndex = map[string]*Function{}
	kwIndex   = map[string]*Keyword{}
)

func init() {
	for i := range functions {
		funcIndex[functions[i].Name] = &functions[i]
	}
	for i := range keywords {
		kwIndex[keywords[i].Name] = &keywords[i]
	}
}

// Functions returns all built-in functions, sorted by name.
func Functions() []Function { return functions }

// Keywords returns all keywords, sorted by name.
func Keywords() []Keyword { return keywords }

// LookupFunction returns the built-in function with the given name.
func LookupFunction(name string) (*Function, bool) {
	f, ok := funcIndex[name]
	return f, ok
}

// LookupKeyword returns the keyword with the given name (without "&").
func LookupKeyword(name string) (*Keyword, bool) {
	k, ok := kwIndex[name]
	return k, ok
}

// Params returns the parameter names of f's signature, e.g. ["s1","i","s2"]
// for left; a final "..." marks a variable number of further arguments.
func (f *Function) Params() []string {
	i, j := strings.IndexByte(f.Signature, '('), strings.LastIndexByte(f.Signature, ')')
	if i < 0 || j <= i+1 {
		return nil
	}
	var ps []string
	for p := range strings.SplitSeq(f.Signature[i+1:j], ",") {
		if p = strings.TrimSpace(p); p != "" {
			ps = append(ps, p)
		}
	}
	return ps
}

const site = "https://www2.cs.arizona.edu/icon/"

// URL returns the address of f's entry in the online Icon reference.
func (f *Function) URL() string {
	if f.Graphics {
		return site + "docs/ipd281/gipd_a.htm#" + f.Name
	}
	return site + "refernce/" + functionPage(f.Name) + "#" + f.Name
}

// URL returns the address of the online keyword list.
func (k *Keyword) URL() string {
	return site + "refernce/keywlist.htm"
}

// functionPage returns the reference page holding a (non-graphics)
// function: they are split alphabetically over five pages.
func functionPage(name string) string {
	switch c := strings.ToLower(name)[0]; {
	case c <= 'd':
		return "funcad.htm"
	case c <= 'j':
		return "funcej.htm"
	case c <= 'o':
		return "funcko.htm"
	case c <= 'r':
		return "funcpr.htm"
	}
	return "funcsz.htm"
}
