package journal

import (
	"encoding/json"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type Type string

const (
	TypeRunStart    Type = "run_start"
	TypeUser        Type = "user"
	TypeAssistant   Type = "assistant"
	TypeToolStarted Type = "tool_started"
	TypeToolResult  Type = "tool_result"
	TypeRunEnd      Type = "run_end"
)

type Record struct {
	Seq  int             `json:"seq"`
	TS   time.Time       `json:"ts"`
	Type Type            `json:"type"`
	Data json.RawMessage `json:"data"`
}

type RunStart struct {
	RunID     string `json:"run_id"`
	Model     string `json:"model"`
	System    string `json:"system"`
	Dir       string `json:"dir"`
	StepLimit int    `json:"step_limit"`
}

type User struct {
	Text string `json:"text"`
}

type Assistant struct {
	Message provider.Message `json:"message"`
}

type ToolStarted struct {
	Call provider.ToolCall `json:"call"`
}

type ToolResultData struct {
	Result provider.ToolResult `json:"result"`
}

type RunEnd struct {
	Reason string `json:"reason"`
}
