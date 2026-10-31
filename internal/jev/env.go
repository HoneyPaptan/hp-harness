package jev

import "errors"

var ErrNoKey = errors.New("TYPESAFE_API_KEY or OPENROUTER_API_KEY must be set")

func FromEnv(getenv func(string) string) (Client, error) {
	key := getenv("TYPESAFE_API_KEY")
	url := TypeSafeURL
	if key == "" {
		key = getenv("OPENROUTER_API_KEY")
		url = OpenRouterURL
	}
	if key == "" {
		return Client{}, ErrNoKey
	}
	model := getenv("HP_JEV_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return Client{URL: url, Key: key, Model: model}, nil
}
