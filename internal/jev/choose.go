package jev

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

const (
	maxUnchosenTools = 8
	toolThreshold    = 0.2
	chooseTimeout    = 5 * time.Second
	maxChooseState   = 2 << 10
)

var errMissingAnswer = errors.New("jev response is missing an answer")

type Chooser struct {
	client  *Client
	timeout time.Duration
}

type chooseState struct {
	FirstUser     string `json:"first_user_message"`
	LastAssistant string `json:"last_assistant_text"`
	LastTools     string `json:"last_tools_called"`
}

func NewChooser(c *Client) *Chooser {
	return &Chooser{client: c, timeout: chooseTimeout}
}

func (c *Chooser) Choose(ctx context.Context, msgs []provider.Message, defs []provider.ToolDef) []provider.ToolDef {
	if len(defs) <= maxUnchosenTools {
		return defs
	}
	scores, err := c.scoreTools(ctx, msgs, defs)
	if err != nil {
		return defs
	}
	return keepTools(defs, scores)
}

func (c *Chooser) scoreTools(ctx context.Context, msgs []provider.Message, defs []provider.ToolDef) ([]float64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	answers, _, err := c.client.Ask(ctx, buildChooseState(msgs), toolQuestions(defs))
	if err != nil {
		return nil, err
	}
	return toolScores(answers, len(defs))
}

func toolQuestions(defs []provider.ToolDef) map[string]Question {
	qs := make(map[string]Question, len(defs))
	for i, d := range defs {
		qs[questionName(i)] = Noul(toolQuestion(d))
	}
	return qs
}

func toolQuestion(d provider.ToolDef) string {
	return fmt.Sprintf("Will the agent's next step call the tool `%s` (%s)?", d.Name, d.Description)
}

func toolScores(answers Answers, n int) ([]float64, error) {
	scores := make([]float64, n)
	for i := range scores {
		a, ok := answers[questionName(i)]
		if !ok {
			return nil, errMissingAnswer
		}
		scores[i] = a.Noul
	}
	return scores, nil
}

func keepTools(defs []provider.ToolDef, scores []float64) []provider.ToolDef {
	var out []provider.ToolDef
	for i, d := range defs {
		if isPinnedTool(d.Name) || scores[i] >= toolThreshold {
			out = append(out, d)
		}
	}
	return out
}

func isPinnedTool(name string) bool {
	return name == "read" || name == "bash"
}

func buildChooseState(msgs []provider.Message) chooseState {
	return chooseState{
		FirstUser:     cutState(firstUserText(msgs)),
		LastAssistant: cutState(lastAssistantText(msgs)),
		LastTools:     cutState(lastToolNames(msgs)),
	}
}

func firstUserText(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role == provider.RoleUser && m.Text != "" {
			return m.Text
		}
	}
	return ""
}

func lastAssistantText(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == provider.RoleAssistant && msgs[i].Text != "" {
			return msgs[i].Text
		}
	}
	return ""
}

func lastToolNames(msgs []provider.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if len(msgs[i].ToolCalls) > 0 {
			return joinToolNames(msgs[i].ToolCalls)
		}
	}
	return ""
}

func joinToolNames(calls []provider.ToolCall) string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	return strings.Join(names, ", ")
}

func cutState(s string) string {
	return strings.ToValidUTF8(s[:min(len(s), maxChooseState)], "")
}
