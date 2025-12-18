package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func writeJSONLine(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return withCode(exitDB, fmt.Errorf("json encode: %w", err))
	}
	return nil
}
