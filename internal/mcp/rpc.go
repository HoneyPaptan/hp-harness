package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	protocolVersion = "2025-11-25"
	maxMessage      = 16 << 20
	methodNotFound  = -32601
)

var acceptedVersions = map[string]bool{
	protocolVersion: true,
	"2025-06-18":    true,
	"2025-03-26":    true,
	"2024-11-05":    true,
}

type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int64 `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
}

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
}

func (m message) isServerRequest() bool {
	return m.Method != "" && hasID(m.ID)
}

func (m message) isResponse() bool {
	return m.Method == "" && hasID(m.ID)
}

func (m message) outcome() (json.RawMessage, error) {
	if m.Error != nil {
		return nil, m.Error
	}
	return m.Result, nil
}

func hasID(id json.RawMessage) bool {
	return len(id) > 0 && string(id) != "null"
}

func responseID(id json.RawMessage) (int64, bool) {
	n, err := strconv.ParseInt(string(id), 10, 64)
	return n, err == nil
}

func encodeRequest(id int64, method string, params any) ([]byte, error) {
	return json.Marshal(request{JSONRPC: "2.0", ID: &id, Method: method, Params: params})
}

func encodeNotification(method string, params any) ([]byte, error) {
	return json.Marshal(request{JSONRPC: "2.0", Method: method, Params: params})
}

func replyTo(m message) ([]byte, error) {
	if m.Method == "ping" {
		return json.Marshal(struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Result  struct{}        `json:"result"`
		}{JSONRPC: "2.0", ID: m.ID})
	}
	return json.Marshal(struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   rpcError        `json:"error"`
	}{JSONRPC: "2.0", ID: m.ID, Error: rpcError{Code: methodNotFound, Message: "method not found"}})
}
