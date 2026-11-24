package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/phonologus/icontools/internal/format"
)

// client drives a Server over in-memory pipes, as an editor would.
type client struct {
	t      *testing.T
	c      *conn
	nextID int
	notes  chan *message // notifications from the server
	resps  chan *message // responses
	done   chan error    // Run's result
}

func newClient(t *testing.T) *client {
	t.Helper()
	cr, sw := io.Pipe() // server -> client
	sr, cw := io.Pipe() // client -> server
	cl := &client{
		t: t, c: newConn(cr, cw),
		notes: make(chan *message, 100), resps: make(chan *message, 100),
		done: make(chan error, 1),
	}
	go func() {
		cl.done <- Run(sr, sw, io.Discard, Options{})
		sw.Close()
	}()
	go func() {
		for {
			m, err := cl.c.read()
			if err != nil {
				close(cl.notes)
				return
			}
			if m.ID != nil {
				cl.resps <- m
			} else {
				cl.notes <- m
			}
		}
	}()
	return cl
}

func (cl *client) call(method string, params, result any) *rpcError {
	cl.t.Helper()
	cl.nextID++
	id := json.RawMessage(fmt.Sprint(cl.nextID))
	b, _ := json.Marshal(params)
	if err := cl.c.write(&message{ID: &id, Method: method, Params: b}); err != nil {
		cl.t.Fatal(err)
	}
	select {
	case m := <-cl.resps:
		if string(*m.ID) != string(id) {
			cl.t.Fatalf("%s: response id %s, want %s", method, *m.ID, id)
		}
		if m.Error != nil {
			return m.Error
		}
		if result != nil {
			if err := json.Unmarshal(m.Result, result); err != nil {
				cl.t.Fatalf("%s: %v in %s", method, err, m.Result)
			}
		}
		return nil
	case <-time.After(5 * time.Second):
		cl.t.Fatalf("%s: no response", method)
	}
	return nil
}

func (cl *client) notify(method string, params any) {
	cl.t.Helper()
	if err := cl.c.notify(method, params); err != nil {
		cl.t.Fatal(err)
	}
}

// diagnostics waits for the next publishDiagnostics notification.
func (cl *client) diagnostics() PublishDiagnosticsParams {
	cl.t.Helper()
	for {
		select {
		case m := <-cl.notes:
			if m == nil {
				cl.t.Fatal("connection closed")
			}
			if m.Method != "textDocument/publishDiagnostics" {
				continue
			}
			var p PublishDiagnosticsParams
			if err := json.Unmarshal(m.Params, &p); err != nil {
				cl.t.Fatal(err)
			}
			return p
		case <-time.After(5 * time.Second):
			cl.t.Fatal("no diagnostics")
		}
	}
}

func (cl *client) initialize(encodings ...string) InitializeResult {
	cl.t.Helper()
	var p InitializeParams
	p.Capabilities.TextDocument.DocumentSymbol.HierarchicalDocumentSymbolSupport = true
	p.Capabilities.General.PositionEncodings = encodings
	var res InitializeResult
	if err := cl.call("initialize", p, &res); err != nil {
		cl.t.Fatal(err)
	}
	cl.notify("initialized", struct{}{})
	return res
}

func (cl *client) shutdown() {
	cl.t.Helper()
	if err := cl.call("shutdown", nil, nil); err != nil {
		cl.t.Fatal(err)
	}
	cl.notify("exit", nil)
	select {
	case err := <-cl.done:
		if err != nil {
			cl.t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		cl.t.Fatal("server did not exit")
	}
}

const uri = "file:///tmp/test.icn"

const broken = `procedure main()
  x := f(a,
end
`

const good = `link strings
global count, total
record point(x,y)

procedure main(args[])
local i
  static n
  every i:=1 to 10 do write(i)
end
`

func TestSession(t *testing.T) {
	cl := newClient(t)
	res := cl.initialize()
	caps := res.Capabilities
	if !caps.DocumentSymbolProvider || !caps.DocumentFormattingProvider || caps.TextDocumentSync.Change != 1 {
		t.Errorf("capabilities: %+v", caps)
	}
	if caps.PositionEncoding != "utf-16" {
		t.Errorf("position encoding %q, want utf-16 by default", caps.PositionEncoding)
	}

	// A syntax error is reported where the parser found it.
	cl.notify("textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "icon", Version: 1, Text: broken},
	})
	p := cl.diagnostics()
	if p.URI != uri || len(p.Diagnostics) == 0 {
		t.Fatalf("diagnostics: %+v", p)
	}
	d := p.Diagnostics[0]
	want := Range{Position{2, 0}, Position{2, 3}} // "end"
	if d.Range != want || d.Severity != SeverityError || !strings.Contains(d.Message, "end") {
		t.Errorf("diagnostic %+v, want range %+v", d, want)
	}

	// Formatting a file with errors does nothing.
	var edits []TextEdit
	if err := cl.call("textDocument/formatting", DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: uri}}, &edits); err != nil {
		t.Fatal(err)
	}
	if len(edits) != 0 {
		t.Errorf("formatting broken file: %v", edits)
	}

	// Fixing the file clears the diagnostics.
	cl.notify("textDocument/didChange", DidChangeTextDocumentParams{
		TextDocument:   VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{{Text: good}},
	})
	if p := cl.diagnostics(); len(p.Diagnostics) != 0 || p.Version == nil || *p.Version != 2 {
		t.Errorf("diagnostics after fix: %+v", p)
	}

	// Document symbols.
	var syms []DocumentSymbol
	if err := cl.call("textDocument/documentSymbol", DocumentSymbolParams{TextDocument: TextDocumentIdentifier{URI: uri}}, &syms); err != nil {
		t.Fatal(err)
	}
	if got, want := outline(syms, ""), `strings(2 link)
count(13 global)
total(13 global)
point(23 record(x, y))
  x(8 field)
  y(8 field)
main(12 procedure(args[]))
  args(13 parameter)
  i(13 local)
  n(13 static)
`; got != want {
		t.Errorf("symbols:\n%s\nwant:\n%s", got, want)
	}
	main := syms[len(syms)-1]
	if main.SelectionRange != (Range{Position{4, 10}, Position{4, 14}}) || main.Range.Start != (Position{4, 0}) || main.Range.End != (Position{8, 3}) {
		t.Errorf("main ranges: %+v", main)
	}

	// Formatting returns an edit that produces icfmt's output.
	if err := cl.call("textDocument/formatting", DocumentFormattingParams{TextDocument: TextDocumentIdentifier{URI: uri}}, &edits); err != nil {
		t.Fatal(err)
	}
	wantOut, _ := format.Source([]byte(good))
	if got := applyEdits(good, edits); got != string(wantOut) {
		t.Errorf("formatted:\n%s\nwant:\n%s", got, wantOut)
	}
	if len(edits) == 1 && edits[0].Range.Start.Line == 0 {
		t.Errorf("edit should skip the unchanged first lines: %+v", edits[0].Range)
	}

	// Unknown requests get MethodNotFound.
	if err := cl.call("textDocument/frobnicate", struct{}{}, nil); err == nil || err.Code != codeMethodNotFound {
		t.Errorf("unknown method: %v", err)
	}

	// Closing clears diagnostics.
	cl.notify("textDocument/didClose", DidCloseTextDocumentParams{TextDocument: TextDocumentIdentifier{URI: uri}})
	if p := cl.diagnostics(); len(p.Diagnostics) != 0 {
		t.Errorf("diagnostics after close: %+v", p)
	}
	cl.shutdown()
}

func TestNotInitialized(t *testing.T) {
	cl := newClient(t)
	if err := cl.call("textDocument/documentSymbol", DocumentSymbolParams{}, nil); err == nil || err.Code != codeServerNotInitialized {
		t.Errorf("before initialize: %v", err)
	}
	cl.initialize()
	cl.shutdown()
}

func TestExitWithoutShutdown(t *testing.T) {
	cl := newClient(t)
	cl.initialize()
	cl.notify("exit", nil)
	if err := <-cl.done; err != ErrExitWithoutShutdown {
		t.Errorf("Run: %v", err)
	}
}

func TestPositions(t *testing.T) {
	// "é" is 2 bytes and 1 UTF-16 unit; "𝄞" is 4 bytes and 2 units.
	src := "# é𝄞\nx := 1\n"
	off := strings.Index(src, "\n") // end of line 0
	m16, m8 := newMapper(src, false), newMapper(src, true)
	if p := m16.position(off); p != (Position{0, 5}) {
		t.Errorf("utf-16 position %+v", p)
	}
	if p := m8.position(off); p != (Position{0, 8}) {
		t.Errorf("utf-8 position %+v", p)
	}
	for _, m := range []*mapper{m16, m8} {
		for i := range len(src) + 1 {
			if i < len(src) && !utf8Start(src[i]) {
				continue
			}
			if got := m.offset(m.position(i)); got != i {
				t.Errorf("utf8=%v: offset(position(%d)) = %d", m.utf8, i, got)
			}
		}
	}
}

func TestUTF8Negotiation(t *testing.T) {
	cl := newClient(t)
	res := cl.initialize("utf-16", "utf-8")
	if res.Capabilities.PositionEncoding != "utf-8" {
		t.Errorf("position encoding %q, want utf-8", res.Capabilities.PositionEncoding)
	}
	cl.shutdown()
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

// outline renders symbols one per line as name(kind detail), indented.
func outline(syms []DocumentSymbol, indent string) string {
	var b strings.Builder
	for _, s := range syms {
		fmt.Fprintf(&b, "%s%s(%d %s)\n", indent, s.Name, s.Kind, s.Detail)
		b.WriteString(outline(s.Children, indent+"  "))
	}
	return b.String()
}

// applyEdits applies non-overlapping edits (UTF-16 positions; ASCII text).
func applyEdits(text string, edits []TextEdit) string {
	m := newMapper(text, false)
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		a, b := m.offset(e.Range.Start), m.offset(e.Range.End)
		text = text[:a] + e.NewText + text[b:]
	}
	return text
}
