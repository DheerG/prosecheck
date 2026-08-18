package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DheerG/prosecheck/internal/modelruntime"
)

func TestFormatTransferProgress(t *testing.T) {
	line := formatTransferProgress(modelruntime.InstallProgress{
		Message: "Downloading the Ministral model", Downloaded: 1300000000,
		Total: 5200000000, BytesPerSecond: 82500000, Transfer: true,
	})
	expected := "Downloading the Ministral model: 1.30 GB / 5.20 GB (25%) at 82.5 MB/s"
	if line != expected {
		t.Fatalf("expected %q, got %q", expected, line)
	}
}

func TestNonTerminalProgressWritesTenPercentSteps(t *testing.T) {
	var output bytes.Buffer
	writer := newInstallProgressWriter(&output)
	for _, downloaded := range []int64{0, 5, 10, 15, 100} {
		writer.Report(modelruntime.InstallProgress{
			Message: "Downloading", Downloaded: downloaded, Total: 100,
			Transfer: true, Done: downloaded == 100,
		})
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected progress at 0, 10, and 100 percent, got %q", output.String())
	}
	if !strings.Contains(lines[0], "(0%)") || !strings.Contains(lines[1], "(10%)") || !strings.Contains(lines[2], "(100%)") {
		t.Fatalf("unexpected progress output %q", output.String())
	}
}
