package main

import (
	"bytes"
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
