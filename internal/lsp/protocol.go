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
	ProcessID             *int                  `json:"processId"`
	RootURI               string                `json:"rootUri"`
	Capabilities          ClientCapabilities    `json:"capabilities"`
	InitializationOptions InitializationOptions `json:"initializationOptions"`
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
	HoverProvider              bool                    `json:"hoverProvider"`
	DefinitionProvider         bool                    `json:"definitionProvider"`
	ReferencesProvider         bool                    `json:"referencesProvider"`
	DocumentHighlightProvider  bool                    `json:"documentHighlightProvider"`
	WorkspaceSymbolProvider    bool                    `json:"workspaceSymbolProvider"`
	CompletionProvider         *CompletionOptions      `json:"completionProvider,omitempty"`
	SignatureHelpProvider      *SignatureHelpOptions   `json:"signatureHelpProvider,omitempty"`
	RenameProvider             *RenameOptions          `json:"renameProvider,omitempty"`
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

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type MarkupContent struct {
	Kind  string `json:"kind"` // "markdown"
	Value string `json:"value"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type ReferenceParams struct {
	TextDocumentPositionParams
	Context struct {
		IncludeDeclaration bool `json:"includeDeclaration"`
	} `json:"context"`
}

type DocumentHighlight struct {
	Range Range `json:"range"`
	Kind  int   `json:"kind"` // 1 text, 2 read, 3 write
}

type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type CompletionItem struct {
	Label         string         `json:"label"`
	Kind          int            `json:"kind"`
	Detail        string         `json:"detail,omitempty"`
	Documentation *MarkupContent `json:"documentation,omitempty"`
	SortText      string         `json:"sortText,omitempty"`
}

// Completion item kinds.
const (
	CompletionFunction = 3
	CompletionField    = 5
	CompletionVariable = 6
	CompletionModule   = 9
	CompletionKeyword  = 14
	CompletionStruct   = 22
	CompletionConstant = 21
)

type SignatureHelpOptions struct {
	TriggerCharacters   []string `json:"triggerCharacters"`
	RetriggerCharacters []string `json:"retriggerCharacters,omitempty"`
}

type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation *MarkupContent         `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters"`
}

type ParameterInformation struct {
	Label string `json:"label"`
}

type RenameOptions struct {
	PrepareProvider bool `json:"prepareProvider"`
}

type RenameParams struct {
	TextDocumentPositionParams
	NewName string `json:"newName"`
}

type PrepareRenameResult struct {
	Range       Range  `json:"range"`
	Placeholder string `json:"placeholder"`
}

type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes"`
}

type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

// InitializationOptions are the iconls-specific settings a client may
// send in initialize.
type InitializationOptions struct {
	// IPath lists directories in which to look for the sources of linked
	// files (e.g. the IPL's procs and gprocs directories).
	IPath []string `json:"ipath"`
}
