package gitutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStagedDiff(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet")
	path := filepath.Join(repository, "message.txt")
	if err := os.WriteFile(path, []byte("durable context\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "message.txt")

	diff, err := stagedDiff(context.Background(), repository, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+durable context") {
		t.Fatalf("unexpected staged diff %q", diff)
	}
}

func TestStagedDiffLimit(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init", "--quiet")
	path := filepath.Join(repository, "message.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("context\n", 20)), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "message.txt")

	diff, err := stagedDiff(context.Background(), repository, 30)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "[diff truncated by prosecheck]") {
		t.Fatalf("expected a truncation marker, got %q", diff)
	}
}

func TestStagedDiffDisabled(t *testing.T) {
	diff, err := StagedDiff(context.Background(), 0)
	if err != nil || diff != "" {
		t.Fatalf("expected an empty diff, got %q and %v", diff, err)
	}
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, output)
	}
}
