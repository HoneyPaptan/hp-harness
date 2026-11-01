package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/mcp"
	"github.com/HoneyPaptan/hp-harness/internal/skill"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

type toolset struct {
	reg     *tool.Registry
	clients []*mcp.Client
}

func openToolset(env Env, skills []skill.Skill, client *jev.Client) *toolset {
	connected, clients := connectionTools(env)
	return &toolset{reg: buildRegistry(skills, jevTools(client), connected, client == nil), clients: clients}
}

func (t *toolset) close() {
	for _, c := range t.clients {
		c.Close()
	}
}

func connectionTools(env Env) ([]tool.Tool, []*mcp.Client) {
	dir, err := userConfigDir(env.Getenv)
	if err != nil {
		return nil, nil
	}
	set, err := loadConnections(env, dir)
	if err != nil {
		warnConnections(env, err)
		return nil, nil
	}
	cache, err := loadSchemaCache(env, dir)
	if err != nil {
		warnConnections(env, err)
		return nil, nil
	}
	return assembleTools(env, set, cache)
}

func warnConnections(env Env, err error) {
	fmt.Fprintf(env.Stderr, "warning: connections not loaded: %v\n", err)
}

func assembleTools(env Env, set connectionSet, cache schemaCache) ([]tool.Tool, []*mcp.Client) {
	var out []tool.Tool
	var clients []*mcp.Client
	for _, name := range slices.Sorted(maps.Keys(set.Servers)) {
		call, client := serverCaller(env, name, set.Servers[name])
		if client != nil {
			clients = append(clients, client)
		}
		for _, c := range cache.Servers[name] {
			out = append(out, connectionTool(c, call))
		}
	}
	return out, clients
}

type remoteCall func(ctx context.Context, remote string, args json.RawMessage) (tool.Result, error)

func serverCaller(env Env, name string, c connection) (remoteCall, *mcp.Client) {
	cfg, err := clientConfig(env, name, c)
	if err != nil {
		failure := fmt.Errorf("mcp %s: %w", name, err)
		return func(context.Context, string, json.RawMessage) (tool.Result, error) {
			return tool.Result{}, failure
		}, nil
	}
	client := mcp.New(cfg)
	return client.Call, client
}

func connectionTool(c cachedTool, call remoteCall) tool.Tool {
	return tool.Tool{
		Name: c.Name, Description: c.Description, Schema: c.Schema,
		Run: func(ctx context.Context, _ tool.Env, raw json.RawMessage) (tool.Result, error) {
			res, err := call(ctx, c.Remote, raw)
			if err == nil {
				return res, nil
			}
			if ctx.Err() != nil {
				return tool.Result{}, ctx.Err()
			}
			return tool.Result{Text: tool.Truncate(err.Error()), IsError: true}, nil
		},
	}
}
