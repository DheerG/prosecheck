package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRequiresEndpointAndModel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := run(nil, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "--endpoint") {
		t.Fatalf("unexpected error %q", stderr.String())
	}
}
