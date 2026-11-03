package permit

import (
	"encoding/json"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type Action struct {
	Tool   string
	Target string
	Kind   Kind
	Linked bool
}

func (a Action) String() string {
	return a.Tool + ": " + a.Target
}

func Describe(call provider.ToolCall, dir string) Action {
	if isMCP(call.Name) {
		return Action{Tool: "mcp", Target: call.Name, Kind: KindExternal}
	}
	switch call.Name {
	case "read":
		return pathAction(call, dir, "read", KindRead)
	case "grep":
		return pathAction(call, dir, "grep", KindRead)
	case "edit":
		return pathAction(call, dir, "edit", KindWrite)
	case "write":
		return pathAction(call, dir, "write", KindWrite)
	case "bash":
		return bashAction(call)
	case "search":
		return searchAction(call)
	case "skill":
		return skillAction(call)
	case "tools":
		return Action{Tool: "tools", Target: "", Kind: KindMeta}
	default:
		return Action{Tool: call.Name, Target: call.Name, Kind: KindExternal}
	}
}

func isMCP(name string) bool {
	return strings.Contains(name, "__")
}

func pathAction(call provider.ToolCall, dir, tool string, kind Kind) Action {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return Action{Tool: tool, Target: "", Kind: kind}
	}
	return Action{Tool: tool, Target: displayPath(dir, args.Path), Kind: kind}
}

func bashAction(call provider.ToolCall) Action {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return Action{Tool: "bash", Target: "", Kind: KindExec}
	}
	return Action{Tool: "bash", Target: strings.TrimSpace(args.Command), Kind: KindExec}
}

func searchAction(call provider.ToolCall) Action {
	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return Action{Tool: "search", Target: "", Kind: KindNet}
	}
	return Action{Tool: "search", Target: args.Query, Kind: KindNet}
}

func skillAction(call provider.ToolCall) Action {
	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(call.Args, &args); err != nil {
		return Action{Tool: "skill", Target: "", Kind: KindMeta}
	}
	return Action{Tool: "skill", Target: args.Name, Kind: KindMeta}
}
