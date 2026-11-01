package mcptest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	EnvMode  = "HP_FAKE_MCP"
	EnvMark  = "HP_FAKE_MARK"
	EnvTools = "HP_FAKE_TOOLS"
	EnvFail  = "HP_FAKE_FAIL"
	EnvEcho  = "HP_FAKE_ECHO_ENV"
	BigSize  = 100_000
)

type message struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Action func(name string, args json.RawMessage) (any, bool)

func Active() bool {
	return os.Getenv(EnvMode) == "1"
}

func Run() {
	mark("start")
	if msg := os.Getenv(EnvFail); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(7)
	}
	fmt.Println("not json, a noisy server line")
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<24)
	for sc.Scan() {
		handle(sc.Bytes())
	}
}

func mark(event string) {
	path := os.Getenv(EnvMark)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %d\n", event, os.Getpid())
}

func handle(line []byte) {
	var m message
	if json.Unmarshal(line, &m) != nil || len(m.ID) == 0 {
		return
	}
	data, _ := json.Marshal(Response(m.ID, m.Method, m.Params, stdioAction))
	fmt.Printf("%s\n", data)
}

func Response(id json.RawMessage, method string, params json.RawMessage, action Action) map[string]any {
	resp := map[string]any{"jsonrpc": "2.0", "id": id}
	result, err := respond(method, params, action)
	if err != nil {
		resp["error"] = map[string]any{"code": -32000, "message": err.Error()}
		return resp
	}
	resp["result"] = result
	return resp
}

func respond(method string, params json.RawMessage, action Action) (any, error) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]string{"name": "fake", "version": "1"},
		}, nil
	case "tools/list":
		return listPage(params), nil
	case "tools/call":
		return callTool(params, action), nil
	}
	return nil, fmt.Errorf("unknown method %s", method)
}

func toolNames() []string {
	names := []string{"echo", "crash", "hang", "big", "fail", "image", "group", "env", "first.crash"}
	n, _ := strconv.Atoi(os.Getenv(EnvTools))
	if n > 0 {
		names = names[:1]
		for i := 1; i < n; i++ {
			names = append(names, fmt.Sprintf("tool%02d", i))
		}
	}
	return names
}

func listPage(params json.RawMessage) map[string]any {
	var p struct {
		Cursor string `json:"cursor"`
	}
	_ = json.Unmarshal(params, &p)
	names := toolNames()
	start, _ := strconv.Atoi(p.Cursor)
	end := min(start+2, len(names))
	tools := make([]map[string]any, 0, end-start)
	for _, name := range names[start:end] {
		tools = append(tools, map[string]any{
			"name": name, "description": "fake tool " + name,
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
		})
	}
	page := map[string]any{"tools": tools}
	if end < len(names) {
		page["nextCursor"] = strconv.Itoa(end)
	}
	return page
}

func callTool(params json.RawMessage, action Action) any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(params, &p)
	if action != nil {
		if out, ok := action(p.Name, p.Arguments); ok {
			return out
		}
	}
	return commonAction(p.Name, p.Arguments)
}

func TextResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func commonAction(name string, args json.RawMessage) any {
	switch name {
	case "big":
		return TextResult(strings.Repeat("x", BigSize), false)
	case "fail":
		return TextResult("tool said no", true)
	case "image":
		return map[string]any{"content": []map[string]any{
			{"type": "text", "text": "before"},
			{"type": "image", "mimeType": "image/png", "data": "QUJDRA=="},
		}}
	}
	return TextResult(fmt.Sprintf("%s:%s", name, args), false)
}

func stdioAction(name string, _ json.RawMessage) (any, bool) {
	switch name {
	case "crash":
		fmt.Fprintln(os.Stderr, "fake crash")
		os.Exit(9)
	case "hang":
		time.Sleep(time.Hour)
	case "first.crash":
		return firstCrash(), true
	case "group":
		return spawnChild(), true
	case "env":
		return TextResult(os.Getenv(os.Getenv(EnvEcho)), false), true
	}
	return nil, false
}

func firstCrash() any {
	data, _ := os.ReadFile(os.Getenv(EnvMark))
	if strings.Count(string(data), "start") < 2 {
		os.Exit(9)
	}
	return TextResult("recovered", false)
}

func spawnChild() any {
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		return TextResult(err.Error(), true)
	}
	return TextResult(strconv.Itoa(cmd.Process.Pid), false)
}
