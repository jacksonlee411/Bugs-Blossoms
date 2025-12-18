package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseJSONObjectField(v string) (json.RawMessage, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return json.RawMessage([]byte(`{}`)), nil
	}
	var anyValue any
	if err := json.Unmarshal([]byte(v), &anyValue); err != nil {
		return nil, err
	}
	if _, ok := anyValue.(map[string]any); !ok {
		return nil, fmt.Errorf("expected json object")
	}
	normalized, err := json.Marshal(anyValue)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}
