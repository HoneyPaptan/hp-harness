package cli

import (
	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func openJev(env Env, on bool) (*jev.Client, error) {
	if !on {
		return nil, nil
	}
	return env.NewJev(env.Getenv)
}

func jevTools(c *jev.Client) []tool.Tool {
	if c == nil {
		return nil
	}
	return []tool.Tool{jev.SearchTool(c)}
}

func jevChooser(c *jev.Client) core.ToolChooser {
	if c == nil {
		return nil
	}
	return jev.NewChooser(c)
}
