package jev

const (
	TypeSafeURL   = "https://api.typesafe.ai/v1/systemone"
	OpenRouterURL = "https://openrouter.ai/api/alpha/decisions"
	DefaultModel  = "jev-latest"
)

const (
	maxQuestionsPerReq = 128
	maxConcurrentAsks  = 8
	errBodyLimit       = 2048
)
