package permit

type Kind int

const (
	KindRead Kind = iota
	KindWrite
	KindExec
	KindNet
	KindExternal
	KindMeta
)

func kindForTool(name string) Kind {
	switch name {
	case "read", "grep":
		return KindRead
	case "edit", "write":
		return KindWrite
	case "bash":
		return KindExec
	case "search":
		return KindNet
	case "skill", "tools":
		return KindMeta
	default:
		return KindExternal
	}
}
