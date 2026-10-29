package tool

import (
	"encoding/json"
	"testing"
)

func TestBuiltinSchemas(t *testing.T) {
	for _, tool := range Builtins() {
		var doc map[string]any
		if err := json.Unmarshal(tool.Schema, &doc); err != nil {
			t.Fatalf("%s schema: %v", tool.Name, err)
		}
		if doc["type"] != "object" {
			t.Fatalf("%s type", tool.Name)
		}
		if doc["additionalProperties"] != false {
			t.Fatalf("%s additionalProperties", tool.Name)
		}
		req, _ := doc["required"].([]any)
		if len(req) == 0 {
			t.Fatalf("%s missing required", tool.Name)
		}
	}
}
