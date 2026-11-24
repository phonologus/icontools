package lsp

// The subset of the Language Server Protocol (3.17) that iconls uses.

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type InitializeParams struct {
	ProcessID    *int               `json:"processId"`
	RootURI      string             `json:"rootUri"`
	Capabilities ClientCapabilities `json:"capabilities"`
}

type ClientCapabilities struct {
	General struct {
		PositionEncodings []string `json:"positionEncodings"`
	} `json:"general"`
	TextDocument struct {
		DocumentSymbol struct {
			HierarchicalDocumentSymbolSupport bool `json:"hierarchicalDocumentSymbolSupport"`
		} `json:"documentSymbol"`
	} `json:"textDocument"`
}

type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
	ServerInfo   ServerInfo         `json:"serverInfo"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ServerCapabilities struct {
	PositionEncoding           string                  `json:"positionEncoding,omitempty"`
	TextDocumentSync           TextDocumentSyncOptions `json:"textDocumentSync"`
	DocumentSymbolProvider     bool                    `json:"documentSymbolProvider"`
	DocumentFormattingProvider bool                    `json:"documentFormattingProvider"`
}

type TextDocumentSyncOptions struct {
	OpenClose bool `json:"openClose"`
	Change    int  `json:"change"` // 1 = full document
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// TextDocumentContentChangeEvent is a full-text change (we ask for full
// sync, so Range is never set).
type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1 error, 2 warning, 3 info, 4 hint
	Source   string `json:"source"`
	Message  string `json:"message"`
}

const (
	SeverityError   = 1
	SeverityWarning = 2
)

type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation is the flat form of document symbol, for clients
// without hierarchical support.
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// Symbol kinds.
const (
	SymbolModule   = 2
	SymbolField    = 8
	SymbolFunction = 12
	SymbolVariable = 13
	SymbolStruct   = 23
)

type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}
