package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/DheerG/prosecheck/internal/checker"
)

type fakeReviewer struct {
	results map[string][]checker.Finding
	errors  map[string]error
}

func (f fakeReviewer) Review(_ context.Context, message, _ string) ([]checker.Finding, error) {
	return f.results[message], f.errors[message]
}

func TestRunCalculatesClassificationMetrics(t *testing.T) {
	suite := Suite{Version: 1, Cases: []Case{
		{ID: "clear", Category: "clear", Message: "clear", ExpectedCodes: []string{}},
		{ID: "missing", Category: "reason", Message: "missing", ExpectedCodes: []string{"SEM002"}},
		{ID: "mixed", Category: "mixed", Message: "mixed", ExpectedCodes: []string{"SEM001", "SEM006"}},
	}}
	reviewer := fakeReviewer{results: map[string][]checker.Finding{
		"clear":   {{Code: "SEM004"}},
		"missing": {{Code: "SEM002"}},
		"mixed":   {{Code: "SEM001"}},
	}}

	report := Run(context.Background(), suite, reviewer, Options{Model: "test", Repetitions: 1})
	if report.TruePositives != 2 || report.FalsePositives != 1 || report.FalseNegatives != 1 {
		t.Fatalf("unexpected counts: %#v", report)
	}
	if report.ExactMatches != 1 || report.ClearFalsePositiveRate != 1 {
		t.Fatalf("unexpected match rates: %#v", report)
	}
	if report.F1 < 0.66 || report.F1 > 0.67 {
		t.Fatalf("unexpected F1 %.3f", report.F1)
	}
	if report.Codes["SEM002"].F1 != 1 || report.Codes["SEM006"].Recall != 0 {
		t.Fatalf("unexpected code scores: %#v", report.Codes)
	}
}

func TestRunCountsProtocolErrorsAsMisses(t *testing.T) {
	suite := Suite{Version: 1, Cases: []Case{
		{ID: "error", Category: "reason", Message: "error", ExpectedCodes: []string{"SEM002"}},
	}}
	reviewer := fakeReviewer{errors: map[string]error{"error": errors.New("bad output")}}

	report := Run(context.Background(), suite, reviewer, Options{})
	if report.ProtocolErrors != 1 || report.FalseNegatives != 1 || report.Results[0].Error == "" {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestValidateRejectsDuplicateIDs(t *testing.T) {
	suite := Suite{Version: 1, Cases: []Case{
		{ID: "same", Category: "clear", Message: "one"},
		{ID: "same", Category: "clear", Message: "two"},
	}}
	if err := suite.Validate(); err == nil {
		t.Fatal("expected duplicate IDs to fail")
	}
}
