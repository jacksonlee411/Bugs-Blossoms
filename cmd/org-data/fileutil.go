package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func ensureDir(path string) error {
	if stringsTrim(path) == "" {
		return withCode(exitUsage, fmt.Errorf("--output is required"))
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return withCode(exitDB, fmt.Errorf("mkdir %s: %w", path, err))
	}
	return nil
}

func writeJSONFile(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return withCode(exitDB, fmt.Errorf("mkdir %s: %w", dir, err))
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return withCode(exitDB, fmt.Errorf("json marshal: %w", err))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return withCode(exitDB, fmt.Errorf("write %s: %w", path, err))
	}
	return nil
}

func readJSONFile(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return withCode(exitUsage, fmt.Errorf("read %s: %w", path, err))
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return withCode(exitValidation, fmt.Errorf("decode %s: %w", path, err))
	}
	return nil
}
