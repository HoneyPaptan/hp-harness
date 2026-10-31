package jev

import "encoding/json"

type wireRequest struct {
	Model     string                  `json:"model"`
	State     any                     `json:"state"`
	Questions map[string]wireQuestion `json:"questions"`
}

type wireQuestion struct {
	Type         string `json:"type"`
	Instructions any    `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type wireResponse struct {
	Model   string                `json:"model"`
	Answers map[string]wireAnswer `json:"answers"`
	Usage   wireUsage             `json:"usage"`
}

type wireAnswer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Confidence    float64            `json:"confidence,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

type wireUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func encodeRequest(model string, state any, qs map[string]Question) ([]byte, error) {
	wqs := make(map[string]wireQuestion, len(qs))
	for name, q := range qs {
		wqs[name] = wireQuestion{
			Type:         q.Type,
			Instructions: q.Instructions,
			Criteria:     q.Criteria,
		}
	}
	return json.Marshal(wireRequest{Model: model, State: state, Questions: wqs})
}

func decodeResponse(body []byte) (Answers, Usage, error) {
	var w wireResponse
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, Usage{}, err
	}
	answers := make(Answers, len(w.Answers))
	for name, a := range w.Answers {
		answers[name] = Answer{
			Type:       a.Type,
			Noul:       a.Noul,
			Choice:     a.Choice,
			Confidence: a.Confidence,
		}
	}
	usage := Usage{InputTokens: w.Usage.InputTokens, OutputTokens: w.Usage.OutputTokens}
	return answers, usage, nil
}
