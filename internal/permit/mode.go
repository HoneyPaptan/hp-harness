package permit

type Mode string

const (
	ModeAsk  Mode = "ask"
	ModeAuto Mode = "auto"
	ModeFull Mode = "full-access"
)

func ParseMode(s string) (Mode, error) {
	switch Mode(s) {
	case ModeAsk, ModeAuto, ModeFull:
		return Mode(s), nil
	default:
		return "", errBadMode
	}
}
