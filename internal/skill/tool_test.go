package skill

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func TestSkillTool(t *testing.T) {
	h := host.NewMemory()
	body := "Do the thing."
	h.WriteFile("/s/deploy/SKILL.md", []byte(body))
	sk := Skill{Name: "deploy", Description: "d", Dir: "/s/deploy", Path: "/s/deploy/SKILL.md"}
	tr := NewTool([]Skill{sk})
	env := tool.Env{Host: h, Dir: "/w"}

	t.Run("plain body", func(t *testing.T) {
		res, err := tr.Run(context.Background(), env, json.RawMessage(`{"name":"deploy"}`))
		if err != nil || res.IsError {
			t.Fatal(res, err)
		}
		if !strings.HasPrefix(res.Text, "skill folder: /s/deploy\n\n") {
			t.Fatalf("prefix %q", res.Text)
		}
		if !strings.HasSuffix(res.Text, body) {
			t.Fatal(res.Text)
		}
	})

	t.Run("no frontmatter", func(t *testing.T) {
		h.WriteFile("/s/plain/SKILL.md", []byte("raw text"))
		sk2 := Skill{Name: "plain", Description: "p", Dir: "/s/plain", Path: "/s/plain/SKILL.md"}
		tr2 := NewTool([]Skill{sk2})
		res, err := tr2.Run(context.Background(), env, json.RawMessage(`{"name":"plain"}`))
		if err != nil || res.IsError {
			t.Fatal(res, err)
		}
		if !strings.Contains(res.Text, "raw text") {
			t.Fatal(res.Text)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		res, err := tr.Run(context.Background(), env, json.RawMessage(`{"name":"nope"}`))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || res.Text != "unknown skill nope; available: deploy" {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("bad args", func(t *testing.T) {
		res, err := tr.Run(context.Background(), env, json.RawMessage(`{"name":1}`))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError || res.Text == "" {
			t.Fatalf("%+v", res)
		}
	})
}
