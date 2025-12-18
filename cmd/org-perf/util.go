package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"
)

func parseScale(raw string) (int, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, errors.New("scale is required")
	}
	switch raw {
	case "1k", "1000":
		return 1000, nil
	default:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("invalid scale: %q", raw)
		}
		if n <= 0 {
			return 0, fmt.Errorf("invalid scale: %q", raw)
		}
		return n, nil
	}
}

func parseEffectiveDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func percentileMillis(samples []float64, p float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	if p <= 0 {
		return samples[0]
	}
	if p >= 1 {
		return samples[len(samples)-1]
	}
	idx := int(float64(len(samples)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(samples) {
		idx = len(samples) - 1
	}
	return samples[idx]
}

func percentiles(samples []float64) (float64, float64, float64) {
	if len(samples) == 0 {
		return 0, 0, 0
	}
	cp := append([]float64(nil), samples...)
	sort.Float64s(cp)
	return percentileMillis(cp, 0.50), percentileMillis(cp, 0.95), percentileMillis(cp, 0.99)
}

func detectGitRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && strings.TrimSpace(setting.Value) != "" {
				return setting.Value
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ensureDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("output path is required")
	}
	dir := path
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return nil
	}
	if idx := strings.LastIndex(dir, string(os.PathSeparator)); idx >= 0 {
		dir = dir[:idx]
	}
	if dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
