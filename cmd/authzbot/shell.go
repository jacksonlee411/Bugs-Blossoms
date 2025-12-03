package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

func runMake(ctx context.Context, root, target string) error {
	if target == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "make", target)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("make %s: %w", target, err)
	}
	return nil
}
