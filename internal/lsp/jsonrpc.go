package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

// message is a JSON-RPC 2.0 request, response or notification.
type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// JSON-RPC and LSP error codes.
const (
	codeParseError           = -32700
	codeInvalidRequest       = -32600
	codeMethodNotFound       = -32601
	codeInvalidParams        = -32602
	codeInternalError        = -32603
	codeServerNotInitialized = -32002
	codeRequestFailed        = -32803
)

// conn reads and writes LSP base-protocol messages: a header block with a
// Content-Length, then that many bytes of JSON.
type conn struct {
	r  *bufio.Reader
	mu sync.Mutex
	w  io.Writer
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReader(r), w: w}
}

func (c *conn) read() (*message, error) {
	hdr, err := textproto.NewReader(c.r).ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(hdr.Get("Content-Length")))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("bad Content-Length %q", hdr.Get("Content-Length"))
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, err
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, &rpcError{codeParseError, err.Error()}
	}
	return &m, nil
}

func (c *conn) write(m *message) error {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

func (c *conn) reply(id *json.RawMessage, result any, err error) error {
	m := &message{ID: id}
	if err != nil {
		re, ok := err.(*rpcError)
		if !ok {
			re = &rpcError{codeRequestFailed, err.Error()}
		}
		m.Error = re
	} else {
		b, merr := json.Marshal(result)
		if merr != nil {
			return merr
		}
		m.Result = b
	}
	return c.write(m)
}

func (c *conn) notify(method string, params any) error {
	b, err := json.Marshal(params)
	if err != nil {
		return err
	}
	return c.write(&message{Method: method, Params: b})
}
