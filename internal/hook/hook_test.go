package hook

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallAndUninstallPlainHook(t *testing.T) {
	repository := newRepository(t)
	path, err := Install(repository)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), managedFileMarker) {
		t.Fatalf("hook does not contain marker: %s", data)
	}
	assertExecutable(t, path)

	secondPath, err := Install(repository)
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	if secondPath != path {
		t.Fatalf("expected %q, got %q", path, secondPath)
	}

	removedPath, err := Uninstall(repository)
	if err != nil {
		t.Fatal(err)
	}
	if removedPath != path {
		t.Fatalf("expected %q, got %q", path, removedPath)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected hook to be removed, got %v", err)
	}
}

func TestInstallDoesNotReplaceAnotherHook(t *testing.T) {
	repository := newRepository(t)
	path := filepath.Join(repository, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(repository); err == nil {
		t.Fatal("expected install to refuse an existing hook")
	} else if !strings.Contains(err.Error(), "Add `prosecheck check") {
		t.Fatalf("expected an integration instruction, got %v", err)
	}
}

func TestInstallAddsBlockToExistingHuskyHook(t *testing.T) {
	repository := newRepository(t)
	runGit(t, repository, "config", "core.hooksPath", ".husky/_")
	path := filepath.Join(repository, ".husky", "commit-msg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "commitlint --edit \"$1\"\n"
	if err := os.WriteFile(path, []byte(original), 0o755); err != nil {
		t.Fatal(err)
	}

	installedPath, err := Install(repository)
	if err != nil {
		t.Fatal(err)
	}
	if installedPath != path {
		t.Fatalf("expected %q, got %q", path, installedPath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, original) || !strings.Contains(content, blockStart) {
		t.Fatalf("Husky hook lost existing content or the prosecheck block: %s", data)
	}
	if strings.Count(content, blockStart) != 1 {
		t.Fatalf("expected one prosecheck block: %s", data)
	}
	assertExecutable(t, path)

	if _, err := Install(repository); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), blockStart) != 1 {
		t.Fatalf("second install added another block: %s", data)
	}
	generatedPath := filepath.Join(repository, ".husky", "_", "commit-msg")
	if _, err := os.Stat(generatedPath); !os.IsNotExist(err) {
		t.Fatalf("expected the generated Husky path to stay untouched, got %v", err)
	}

	if _, err := Uninstall(repository); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("expected the original Husky hook, got %q", data)
	}
}

func TestInstallCreatesAndRemovesHuskyHook(t *testing.T) {
	repository := newRepository(t)
	if err := os.MkdirAll(filepath.Join(repository, ".husky"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := Install(repository)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Dir(path)) != ".husky" {
		t.Fatalf("expected a Husky hook path, got %q", path)
	}
	if _, err := Uninstall(repository); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected hook to be removed, got %v", err)
	}
}

func TestInstallGivesInstructionsForConfigurationManagers(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		manager     Manager
		instruction string
	}{
		{"Lefthook", "lefthook.yml", ManagerLefthook, "lefthook install"},
		{"pre-commit", ".pre-commit-config.yaml", ManagerPreCommit, "--hook-type commit-msg"},
		{"Overcommit", ".overcommit.yml", ManagerOvercommit, "overcommit --install"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newRepository(t)
			configPath := filepath.Join(repository, test.config)
			if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(configPath, []byte("# existing configuration\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := Install(repository)
			var integrationErr *IntegrationError
			if !errors.As(err, &integrationErr) {
				t.Fatalf("expected IntegrationError, got %v", err)
			}
			if integrationErr.Manager != test.manager {
				t.Fatalf("expected %s, got %s", test.manager, integrationErr.Manager)
			}
			if !strings.Contains(err.Error(), test.instruction) {
				t.Fatalf("expected %q in %q", test.instruction, err)
			}
			hookPath := filepath.Join(repository, ".git", "hooks", "commit-msg")
			if _, statErr := os.Stat(hookPath); !os.IsNotExist(statErr) {
				t.Fatalf("expected no generated hook, got %v", statErr)
			}
		})
	}
}

func TestInstallDetectsGeneratedPreCommitHook(t *testing.T) {
	repository := newRepository(t)
	path := filepath.Join(repository, ".git", "hooks", "commit-msg")
	content := "#!/usr/bin/env bash\n# File generated by pre-commit: https://pre-commit.com\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Install(repository)
	var integrationErr *IntegrationError
	if !errors.As(err, &integrationErr) {
		t.Fatalf("expected IntegrationError, got %v", err)
	}
	if integrationErr.Manager != ManagerPreCommit {
		t.Fatalf("expected pre-commit, got %s", integrationErr.Manager)
	}
}

func TestInstallUsesCustomGitHooksPath(t *testing.T) {
	repository := newRepository(t)
	runGit(t, repository, "config", "core.hooksPath", ".githooks")

	path, err := Install(repository)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(repository, ".githooks", "commit-msg")
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
}

func TestUninstallFindsManagedHookAfterManagerConfigIsAdded(t *testing.T) {
	repository := newRepository(t)
	path, err := Install(repository)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(repository, ".pre-commit-config.yaml")
	if err := os.WriteFile(configPath, []byte("repos: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	removedPath, err := Uninstall(repository)
	if err != nil {
		t.Fatal(err)
	}
	if removedPath != path {
		t.Fatalf("expected %q, got %q", path, removedPath)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected hook to be removed, got %v", err)
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("hook is not executable: %s", info.Mode())
	}
}

func newRepository(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	runGit(t, "", "init", "--quiet", directory)
	realDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	return realDirectory
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	if directory != "" {
		command.Dir = directory
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, output)
	}
}
