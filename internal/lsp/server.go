package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"

	"github.com/phonologus/icontools/internal/builtin"
	"github.com/phonologus/icontools/internal/format"
	"github.com/phonologus/icontools/internal/syntax"
	"github.com/phonologus/icontools/internal/token"
)

// Version is reported to clients in serverInfo.
const Version = "0.1.0"

// Server is an Icon language server. It handles one client over one
// connection, processing messages in order.
type Server struct {
	opts         Options
	conn         *conn
	log          *log.Logger
	docs         map[string]*document
	initialized  bool
	shutdown     bool
	utf8         bool // positions are UTF-8 byte offsets (else UTF-16)
	hierarchical bool // client supports hierarchical document symbols
}

// document is an open text document and its parse.
type document struct {
	uri     string
	version int
	text    string
	file    *syntax.File
	m       *mapper
}

// Options configures a Server.
type Options struct {
	Docs *builtin.Docs // reference documentation, or nil
}

// ErrExitWithoutShutdown is returned by Run if the client sent "exit"
// without first sending "shutdown"; the process should exit with status 1.
var ErrExitWithoutShutdown = errors.New("exit without shutdown")

// Run serves the client on r and w until it sends "exit" or r is closed.
// Diagnostic logging goes to logw (which may be io.Discard).
func Run(r io.Reader, w io.Writer, logw io.Writer, opts Options) error {
	s := &Server{
		opts: opts,
		conn: newConn(r, w),
		log:  log.New(logw, "iconls: ", log.LstdFlags),
		docs: map[string]*document{},
	}
	for {
		m, err := s.conn.read()
		if err != nil {
			var re *rpcError
			if errors.As(err, &re) {
				s.conn.reply(nil, nil, re)
				continue
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if m.Method == "exit" {
			if !s.shutdown {
				return ErrExitWithoutShutdown
			}
			return nil
		}
		s.handle(m)
	}
}

func (s *Server) handle(m *message) {
	isRequest := m.ID != nil
	defer func() {
		if r := recover(); r != nil {
			s.log.Printf("panic handling %s: %v", m.Method, r)
			if isRequest {
				s.conn.reply(m.ID, nil, &rpcError{codeInternalError, fmt.Sprint(r)})
			}
		}
	}()
	if !s.initialized && m.Method != "initialize" {
		if isRequest {
			s.conn.reply(m.ID, nil, &rpcError{codeServerNotInitialized, "server not initialized"})
		}
		return
	}
	result, err := s.dispatch(m)
	if isRequest {
		if err := s.conn.reply(m.ID, result, err); err != nil {
			s.log.Printf("reply to %s: %v", m.Method, err)
		}
	} else if err != nil {
		s.log.Printf("%s: %v", m.Method, err)
	}
}

func (s *Server) dispatch(m *message) (any, error) {
	switch m.Method {
	case "initialize":
		var p InitializeParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		return s.initialize(&p), nil
	case "initialized":
		return nil, nil
	case "shutdown":
		s.shutdown = true
		return nil, nil
	case "textDocument/didOpen":
		var p DidOpenTextDocumentParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		s.update(p.TextDocument.URI, p.TextDocument.Version, p.TextDocument.Text)
		return nil, nil
	case "textDocument/didChange":
		var p DidChangeTextDocumentParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		if n := len(p.ContentChanges); n > 0 {
			s.update(p.TextDocument.URI, p.TextDocument.Version, p.ContentChanges[n-1].Text)
		}
		return nil, nil
	case "textDocument/didClose":
		var p DidCloseTextDocumentParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		delete(s.docs, p.TextDocument.URI)
		// Clear the diagnostics of the closed document.
		return nil, s.conn.notify("textDocument/publishDiagnostics",
			PublishDiagnosticsParams{URI: p.TextDocument.URI, Diagnostics: []Diagnostic{}})
	case "textDocument/documentSymbol":
		var p DocumentSymbolParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		d, err := s.doc(p.TextDocument.URI)
		if err != nil {
			return nil, err
		}
		syms := documentSymbols(d)
		if s.hierarchical {
			return syms, nil
		}
		return flatten(d.uri, syms, ""), nil
	case "textDocument/formatting":
		var p DocumentFormattingParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		d, err := s.doc(p.TextDocument.URI)
		if err != nil {
			return nil, err
		}
		return formatting(d), nil
	}
	if m.ID != nil && !strings.HasPrefix(m.Method, "$/") {
		return nil, &rpcError{codeMethodNotFound, "method not supported: " + m.Method}
	}
	return nil, nil // unknown notifications are ignored
}

func unmarshal(b json.RawMessage, v any) error {
	if err := json.Unmarshal(b, v); err != nil {
		return &rpcError{codeInvalidParams, err.Error()}
	}
	return nil
}

func (s *Server) initialize(p *InitializeParams) *InitializeResult {
	s.initialized = true
	s.hierarchical = p.Capabilities.TextDocument.DocumentSymbol.HierarchicalDocumentSymbolSupport
	res := &InitializeResult{
		ServerInfo: ServerInfo{Name: "iconls", Version: Version},
		Capabilities: ServerCapabilities{
			PositionEncoding:           "utf-16",
			TextDocumentSync:           TextDocumentSyncOptions{OpenClose: true, Change: 1},
			DocumentSymbolProvider:     true,
			DocumentFormattingProvider: true,
		},
	}
	for _, e := range p.Capabilities.General.PositionEncodings {
		if e == "utf-8" {
			s.utf8 = true
			res.Capabilities.PositionEncoding = "utf-8"
		}
	}
	return res
}

func (s *Server) doc(uri string) (*document, error) {
	d := s.docs[uri]
	if d == nil {
		return nil, &rpcError{codeInvalidParams, "document not open: " + uri}
	}
	return d, nil
}

// update records new text for a document, reparses it and publishes its
// diagnostics.
func (s *Server) update(uri string, version int, text string) {
	d := &document{uri: uri, version: version, text: text}
	d.file = syntax.Parse(text)
	d.m = newMapper(text, s.utf8)
	s.docs[uri] = d
	v := version
	if err := s.conn.notify("textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI: uri, Version: &v, Diagnostics: diagnostics(d),
	}); err != nil {
		s.log.Printf("publishDiagnostics: %v", err)
	}
}

// ---------------------------------------------------------------------
// Diagnostics

func diagnostics(d *document) []Diagnostic {
	ds := []Diagnostic{}
	for _, e := range d.file.Errors {
		start, end := e.Offset, e.Offset
		if t := tokenAt(d.file, e.Offset); t != nil {
			end = t.End()
		}
		ds = append(ds, Diagnostic{
			Range:    d.m.rangeOf(start, end),
			Severity: SeverityError,
			Source:   "iconls",
			Message:  e.Msg,
		})
	}
	return ds
}

// tokenAt returns the non-implicit token starting at byte offset off, if
// any.
func tokenAt(f *syntax.File, off int) *syntax.Token {
	toks := f.Tokens
	i := sort.Search(len(toks), func(i int) bool { return toks[i].Offset >= off })
	for ; i < len(toks) && toks[i].Offset == off; i++ {
		if !toks[i].Implicit {
			return &toks[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Formatting

// formatting returns the edit that formats d, or no edits if d has syntax
// errors (as gofmt, icfmt will not format code it cannot parse). The edit
// replaces only the lines that differ.
func formatting(d *document) []TextEdit {
	if len(d.file.Errors) > 0 {
		return []TextEdit{}
	}
	out := format.File(d.file)
	if out == d.text {
		return []TextEdit{}
	}
	// Trim the common prefix and suffix, on line boundaries.
	a, b := d.text, out
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	p = strings.LastIndexByte(a[:p], '\n') + 1
	q := 0
	for q < len(a)-p && q < len(b)-p && a[len(a)-1-q] == b[len(b)-1-q] {
		q++
	}
	if i := strings.IndexByte(a[len(a)-q:], '\n'); i >= 0 {
		q -= i + 1
	} else {
		q = 0
	}
	return []TextEdit{{
		Range:   d.m.rangeOf(p, len(a)-q),
		NewText: b[p : len(b)-q],
	}}
}

// ---------------------------------------------------------------------
// Document symbols

func documentSymbols(d *document) []DocumentSymbol {
	syms := []DocumentSymbol{}
	name := func(t *syntax.Token, kind int, detail string, whole syntax.Node) (DocumentSymbol, bool) {
		if t == nil || t.Kind == token.ILLEGAL {
			return DocumentSymbol{}, false
		}
		sel := d.m.rangeOf(t.Offset, t.End())
		rng := sel
		if whole != nil {
			if a, b, ok := syntax.Span(whole); ok {
				rng = d.m.rangeOf(a, b)
			}
		}
		return DocumentSymbol{Name: unquote(t.Text), Detail: detail, Kind: kind, Range: rng, SelectionRange: sel}, true
	}
	for _, decl := range d.file.Decls {
		switch decl := decl.(type) {
		case *syntax.LinkDecl:
			for _, t := range decl.Files.Elems {
				if s, ok := name(t, SymbolModule, "link", nil); ok {
					syms = append(syms, s)
				}
			}
		case *syntax.GlobalDecl:
			for _, t := range decl.Names.Elems {
				if s, ok := name(t, SymbolVariable, "global", nil); ok {
					syms = append(syms, s)
				}
			}
		case *syntax.RecordDecl:
			s, ok := name(decl.Name, SymbolStruct, "record"+paramList(decl.Fields.Elems, false), decl)
			if !ok {
				continue
			}
			for _, f := range decl.Fields.Elems {
				if c, ok := name(f, SymbolField, "field", nil); ok {
					s.Children = append(s.Children, c)
				}
			}
			syms = append(syms, s)
		case *syntax.ProcDecl:
			s, ok := name(decl.Name, SymbolFunction,
				"procedure"+paramList(decl.Params.Elems, decl.Lbrack != nil), decl)
			if !ok {
				continue
			}
			for _, t := range decl.Params.Elems {
				if c, ok := name(t, SymbolVariable, "parameter", nil); ok {
					s.Children = append(s.Children, c)
				}
			}
			for _, l := range decl.Locals {
				for _, t := range l.Names.Elems {
					if c, ok := name(t, SymbolVariable, l.Keyword.Text, nil); ok {
						s.Children = append(s.Children, c)
					}
				}
			}
			syms = append(syms, s)
		}
	}
	return syms
}

// paramList renders "(a, b, c[])".
func paramList(ts []*syntax.Token, variadic bool) string {
	var names []string
	for _, t := range ts {
		if t != nil {
			names = append(names, t.Text)
		}
	}
	s := strings.Join(names, ", ")
	if variadic {
		s += "[]"
	}
	return "(" + s + ")"
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// flatten converts hierarchical symbols to SymbolInformation.
func flatten(uri string, syms []DocumentSymbol, container string) []SymbolInformation {
	out := []SymbolInformation{}
	for _, s := range syms {
		out = append(out, SymbolInformation{
			Name: s.Name, Kind: s.Kind, ContainerName: container,
			Location: Location{URI: uri, Range: s.SelectionRange},
		})
		out = append(out, flatten(uri, s.Children, s.Name)...)
	}
	return out
}
