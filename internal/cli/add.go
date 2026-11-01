package cli

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/mcp"
)

const addUsage = "usage: hp add <folder|git-url> | hp add <name> <https-url> [--header K=V] [--timeout 90s] | hp add <name> [--env K=V] [--timeout 90s] -- <command> [args] | hp add --refresh <name>"

type addArgs struct {
	refresh    string
	positional []string
	headers    map[string]string
	env        map[string]string
	timeout    string
	command    []string
	dashes     bool
}

func cmdAdd(ctx context.Context, env Env, args []string) int {
	a, err := parseAdd(args)
	if err != nil {
		return exitUsage(env, err)
	}
	if err := runAdd(ctx, env, a); err != nil {
		return exitAdd(env, err)
	}
	return 0
}

func exitAdd(env Env, err error) int {
	if _, ok := err.(usageError); ok {
		return exitUsage(env, err)
	}
	return exitErr(env, "", "", err)
}

func parseAdd(args []string) (addArgs, error) {
	a := addArgs{headers: map[string]string{}, env: map[string]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			a.dashes, a.command = true, args[i+1:]
			break
		}
		if !strings.HasPrefix(arg, "-") {
			a.positional = append(a.positional, arg)
			continue
		}
		i++
		if i == len(args) {
			return addArgs{}, usagef("%s needs a value", arg)
		}
		if err := a.flag(arg, args[i]); err != nil {
			return addArgs{}, err
		}
	}
	return a, nil
}

func (a *addArgs) flag(name, value string) error {
	switch name {
	case "--refresh":
		a.refresh = value
		return nil
	case "--header":
		return setPair(a.headers, value)
	case "--env":
		return setPair(a.env, value)
	case "--timeout":
		_, err := parseTimeout(value)
		a.timeout = value
		return err
	}
	return usagef("unknown flag %s\n%s", name, addUsage)
}

func setPair(into map[string]string, pair string) error {
	k, v, ok := strings.Cut(pair, "=")
	if !ok || k == "" {
		return usagef("%q is not K=V", pair)
	}
	into[k] = v
	return nil
}

func runAdd(ctx context.Context, env Env, a addArgs) error {
	if a.refresh != "" {
		return refreshServer(ctx, env, a)
	}
	if a.dashes {
		return addStdio(ctx, env, a)
	}
	if len(a.env) > 0 {
		return usagef("--env needs a command after --\n%s", addUsage)
	}
	switch len(a.positional) {
	case 1:
		if len(a.headers) > 0 || a.timeout != "" {
			return usagef("--header and --timeout need a server\n%s", addUsage)
		}
		return addSkill(ctx, env, a.positional[0])
	case 2:
		return addHTTP(ctx, env, a)
	}
	return usagef("%s", addUsage)
}

func addStdio(ctx context.Context, env Env, a addArgs) error {
	if len(a.positional) != 1 || len(a.command) == 0 || len(a.headers) > 0 {
		return usagef("%s", addUsage)
	}
	return addServer(ctx, env, a.positional[0], connection{
		Command: a.command[0], Args: a.command[1:], Dir: env.Cwd, Env: nonEmpty(a.env), Timeout: a.timeout,
	})
}

func addHTTP(ctx context.Context, env Env, a addArgs) error {
	raw := a.positional[1]
	if err := checkServerURL(raw); err != nil {
		return err
	}
	return addServer(ctx, env, a.positional[0], connection{URL: raw, Headers: nonEmpty(a.headers), Timeout: a.timeout})
}

func nonEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

func checkServerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return usagef("%q is not a URL", raw)
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname())) {
		return nil
	}
	return usagef("server URL must use https, or http on a loopback address")
}

func addServer(ctx context.Context, env Env, name string, c connection) error {
	if err := checkName(name); err != nil {
		return err
	}
	dir, err := userConfigDir(env.Getenv)
	if err != nil {
		return err
	}
	set, err := loadConnections(env, dir)
	if err != nil {
		return err
	}
	if taken, err := nameTaken(env, dir, set, name); err != nil || taken {
		return alreadyAdded(name, err)
	}
	tools, err := probeServer(ctx, env, name, c)
	if err != nil {
		return err
	}
	if err := storeTools(env, dir, name, tools); err != nil {
		return err
	}
	set.Servers[name] = c
	if err := saveConnections(dir, set); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "added %s: %s\n", name, toolCount(len(tools)))
	return nil
}

func alreadyAdded(name string, err error) error {
	if err != nil {
		return err
	}
	return usagef("%s already exists, use hp remove %s first", name, name)
}

func probeServer(ctx context.Context, env Env, name string, c connection) ([]cachedTool, error) {
	cfg, err := clientConfig(env, name, c)
	if err != nil {
		return nil, fmt.Errorf("mcp %s: %w", name, err)
	}
	client := mcp.New(cfg)
	defer client.Close()
	tools, err := client.Tools(ctx)
	if err != nil {
		return nil, err
	}
	return exposeTools(name, tools), nil
}

func storeTools(env Env, dir, name string, tools []cachedTool) error {
	cache, err := loadSchemaCache(env, dir)
	if err != nil {
		return err
	}
	cache.Servers[name] = tools
	return saveSchemaCache(dir, cache)
}

func refreshServer(ctx context.Context, env Env, a addArgs) error {
	if len(a.positional) > 0 || a.dashes || len(a.headers) > 0 || len(a.env) > 0 || a.timeout != "" {
		return usagef("%s", addUsage)
	}
	dir, err := userConfigDir(env.Getenv)
	if err != nil {
		return err
	}
	set, err := loadConnections(env, dir)
	if err != nil {
		return err
	}
	c, ok := set.Servers[a.refresh]
	if !ok {
		return fmt.Errorf("no connection named %s", a.refresh)
	}
	tools, err := probeServer(ctx, env, a.refresh, c)
	if err != nil {
		return err
	}
	if err := storeTools(env, dir, a.refresh, tools); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "refreshed %s: %s\n", a.refresh, toolCount(len(tools)))
	return nil
}

func toolCount(n int) string {
	if n == 1 {
		return "1 tool"
	}
	return fmt.Sprintf("%d tools", n)
}
