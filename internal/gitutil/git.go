package gitutil

import (
	"context"
	"os/exec"
)

func StagedDiff(ctx context.Context, limit int) (string, error) {
	return stagedDiff(ctx, "", limit)
}

func stagedDiff(ctx context.Context, directory string, limit int) (string, error) {
	if limit == 0 {
		return "", nil
	}
	command := exec.CommandContext(ctx, "git", "diff", "--cached", "--no-ext-diff", "--unified=2", "--")
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	if len(output) <= limit {
		return string(output), nil
	}
	return string(output[:limit]) + "\n[diff truncated by prosecheck]\n", nil
}
