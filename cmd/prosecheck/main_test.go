package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckPassesClearMessage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", "--message", "Prevent duplicate invoice delivery"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "passed") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunCheckStrictFailsOnWarning(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", "--strict", "--message", "Updated account validation"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "PC007") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunCheckWritesJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"check", "--format", "json", "--message", "WIP"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"code": "PC005"`) {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunModelHelpListsCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"model", "help"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "model doctor") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunModelStatusReportsMissingInstall(t *testing.T) {
	t.Setenv("PROSECHECK_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"model", "status"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "not installed") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunModelInstallRejectsUnknownModel(t *testing.T) {
	t.Setenv("PROSECHECK_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"model", "install", "other-model"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "supported model is bonsai-8b") {
		t.Fatalf("unexpected error %q", stderr.String())
	}
}

func TestRunCheckReportsMissingManagedModelAsNote(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PROSECHECK_HOME", filepath.Join(root, "model-data"))
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"semantic":{"enabled":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"check", "--config", configPath, "--message", "Prevent duplicate invoice delivery",
	}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d: %s", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "PC901") || !strings.Contains(stdout.String(), "model install") {
		t.Fatalf("unexpected output %q", stdout.String())
	}
}

func TestRunCheckRequiresManagedModelWhenForced(t *testing.T) {
	t.Setenv("PROSECHECK_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{
		"check", "--semantic", "on", "--message", "Prevent duplicate invoice delivery",
	}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "model install") {
		t.Fatalf("unexpected error %q", stderr.String())
	}
}
