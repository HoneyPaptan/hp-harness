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
	TypeLimitWait   Type = "limit_wait"
	TypeModelSwitch Type = "model_switch"
	TypeRunEnd      Type = "run_end"
)

type Record struct {
	Seq  int             `json:"seq"`
	TS   time.Time       `json:"ts"`
	Type Type            `json:"type"`
	Data json.RawMessage `json:"data"`
}

type RunStart struct {
	RunID       string        `json:"run_id"`
	Model       string        `json:"model"`
	System      string        `json:"system"`
	Dir         string        `json:"dir"`
	StepLimit   int           `json:"step_limit"`
	Preset      string        `json:"preset"`
	BaseURL     string        `json:"base_url"`
	KeyEnv      string        `json:"key_env"`
	MaxTokens   int           `json:"max_tokens"`
	Fallbacks   []string      `json:"fallbacks,omitempty"`
	SwitchAfter time.Duration `json:"switch_after,omitempty"`
	MaxWait     time.Duration `json:"max_wait,omitempty"`
	Jev         bool          `json:"jev,omitempty"`
}

type LimitWait struct {
	Model string    `json:"model"`
	Until time.Time `json:"until"`
}

type ModelSwitch struct {
	From string `json:"from"`
	To   string `json:"to"`
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
