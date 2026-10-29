package skill

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func NewTool(skills []Skill) tool.Tool {
	return tool.Tool{
		Name:        "skill",
		Description: "Load a skill by name. Returns its instructions and its folder path; read referenced files relative to that folder.",
		Schema:      skillSchema,
		Run:         runSkill(skills),
	}
}

var skillSchema = json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"Skill name from the system prompt index."}},"required":["name"],"additionalProperties":false}`)

type skillArgs struct {
	Name string `json:"name"`
}

func runSkill(skills []Skill) func(context.Context, tool.Env, json.RawMessage) (tool.Result, error) {
	byName := make(map[string]Skill, len(skills))
	names := make([]string, len(skills))
	for i, sk := range skills {
		byName[sk.Name] = sk
		names[i] = sk.Name
	}
	slices.Sort(names)
	return func(ctx context.Context, env tool.Env, raw json.RawMessage) (tool.Result, error) {
		args, err := decodeArgs(raw)
		if err != nil {
			return tool.Result{Text: fmt.Sprintf("%v", err), IsError: true}, nil
		}
		sk, ok := byName[args.Name]
		if !ok {
			return tool.Result{Text: fmt.Sprintf("unknown skill %s; available: %s", args.Name, strings.Join(names, ", ")), IsError: true}, nil
		}
		return loadToolResult(env, sk)
	}
}

func decodeArgs(raw json.RawMessage) (skillArgs, error) {
	var v skillArgs
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	err := dec.Decode(&v)
	return v, err
}

func loadToolResult(env tool.Env, sk Skill) (tool.Result, error) {
	data, err := env.Host.ReadFile(sk.Path)
	if err != nil {
		return tool.Result{}, err
	}
	_, body, ok := parseFrontmatter(string(data))
	text := string(data)
	if ok {
		text = body
	}
	text = strings.TrimSpace(text)
	out := fmt.Sprintf("skill folder: %s\n\n%s", sk.Dir, text)
	return tool.Result{Text: out}, nil
}
