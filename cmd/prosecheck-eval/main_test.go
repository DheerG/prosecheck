package main

import (
	"bytes"
	"strings"
	"testing"

	proseeval "github.com/DheerG/prosecheck/internal/eval"
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

func TestWriteTextReportGroupsRepeatedMismatches(t *testing.T) {
	report := proseeval.Report{
		Model: "test", CaseCount: 1, Repetitions: 3, RequestCount: 3,
		Codes: map[string]*proseeval.CodeScore{
			"SEM001": {}, "SEM002": {}, "SEM003": {},
			"SEM004": {}, "SEM005": {}, "SEM006": {},
		},
		Results: []proseeval.CaseResult{
			{ID: "case", ExpectedCodes: []string{"SEM001"}, ActualCodes: []string{"SEM002"}},
			{ID: "case", ExpectedCodes: []string{"SEM001"}, ActualCodes: []string{"SEM002"}},
			{ID: "case", ExpectedCodes: []string{"SEM001"}, ActualCodes: []string{"SEM002"}},
		},
	}
	var output bytes.Buffer
	writeTextReport(&output, report)
	if strings.Count(output.String(), "case (3/3 runs)") != 1 {
		t.Fatalf("expected one grouped mismatch, got %q", output.String())
	}
}
