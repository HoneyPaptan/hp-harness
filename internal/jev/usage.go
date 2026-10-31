package jev

type Usage struct {
	InputTokens  int
	OutputTokens int
}

func (u Usage) add(v Usage) Usage {
	return Usage{
		InputTokens:  u.InputTokens + v.InputTokens,
		OutputTokens: u.OutputTokens + v.OutputTokens,
	}
}
