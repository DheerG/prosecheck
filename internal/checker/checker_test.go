package checker

import (
	"strings"
	"testing"

	"github.com/DheerG/prosecheck/internal/config"
)

func TestCheckAcceptsDurableMessage(t *testing.T) {
	message := `Expire inactive sessions after thirty days

Long-lived sessions increased the risk from unattended devices. The new limit applies after
thirty days without activity.`

	report := Check(message, config.Default())
	if len(report.Findings) != 0 {
		t.Fatalf("expected no findings, got %#v", report.Findings)
	}
}

func TestCheckFindsCommonDrift(t *testing.T) {
	message := `Updated code.
Technical issues identified:
- Changed the timeout and several helper functions.`

	report := Check(message, config.Default())
	codes := findingCodes(report.Findings)
	for _, expected := range []string{"PC004", "PC006", "PC007", "PC008", "PC009"} {
		if !strings.Contains(codes, expected) {
			t.Errorf("expected %s in %s", expected, codes)
		}
	}
}

func TestCheckRejectsPlaceholder(t *testing.T) {
	report := Check("WIP\n", config.Default())
	if !report.Failed(false) {
		t.Fatal("expected a placeholder subject to fail")
	}
	if got := report.Findings[0].Code; got != "PC003" && got != "PC005" {
		t.Fatalf("expected a placeholder finding, got %s", got)
	}
	if !strings.Contains(findingCodes(report.Findings), "PC005") {
		t.Fatalf("expected PC005, got %#v", report.Findings)
	}
}

func TestCheckUnderstandsConventionalPrefix(t *testing.T) {
	report := Check("fix(auth): Reject expired session cookies\n", config.Default())
	if len(report.Findings) != 0 {
		t.Fatalf("expected no findings, got %#v", report.Findings)
	}
}

func TestCheckSkipsGeneratedSubjects(t *testing.T) {
	for _, message := range []string{
		"Merge branch 'main' into feature\n",
		"Revert \"Remove compatibility path\"\n",
		"fixup! Add account lockout policy\n",
	} {
		report := Check(message, config.Default())
		if len(report.Findings) != 0 {
			t.Errorf("expected generated subject %q to pass, got %#v", message, report.Findings)
		}
	}
}

func TestCheckFindsProcessFirstBody(t *testing.T) {
	message := `Prevent duplicate invoice delivery

This commit updates the worker and changes the retry helper.`
	report := Check(message, config.Default())
	if !strings.Contains(findingCodes(report.Findings), "PC010") {
		t.Fatalf("expected PC010, got %#v", report.Findings)
	}
}

func TestCheckFindsLongSentence(t *testing.T) {
	message := `Keep retry state across worker restarts

The worker now saves every pending retry before shutdown because a deployment can otherwise discard queued operations and leave customer invoices in an unknown delivery state.`
	report := Check(message, config.Default())
	if !strings.Contains(findingCodes(report.Findings), "PC012") {
		t.Fatalf("expected PC012, got %#v", report.Findings)
	}
}

func TestCheckAppliesRuleOverride(t *testing.T) {
	cfg := config.Default()
	cfg.Rules["PC004"] = "off"
	cfg.Rules["PC005"] = "warning"

	report := Check("WIP.\n", cfg)
	if strings.Contains(findingCodes(report.Findings), "PC004") {
		t.Fatalf("expected PC004 to be disabled, got %#v", report.Findings)
	}
	for _, finding := range report.Findings {
		if finding.Code == "PC005" && finding.Severity != SeverityWarning {
			t.Fatalf("expected PC005 warning, got %s", finding.Severity)
		}
	}
}

func TestCheckAppliesSimpleEnglishRules(t *testing.T) {
	cfg := config.Default()
	cfg.SimpleEnglish.Enabled = true
	message := `Keep retries available after a restart

It is worth noting that workers shouldn't utilize stale retry state, e.g. after a deployment.
The worker might lose records that have been saved.`

	report := Check(message, cfg)
	codes := findingCodes(report.Findings)
	for _, expected := range []string{"PC015", "PC016", "PC017", "PC018", "PC019"} {
		if !strings.Contains(codes, expected) {
			t.Errorf("expected %s in %s", expected, codes)
		}
	}
}

func TestSimpleEnglishMakesSemicolonsWarnings(t *testing.T) {
	report := Check("Keep retry state\n\nThe worker saves retries; deployments keep them.", config.Default())
	for _, finding := range report.Findings {
		if finding.Code == "PC014" && finding.Severity != SeverityWarning {
			t.Fatalf("expected a semicolon warning, got %s", finding.Severity)
		}
	}
}

func TestCheckSkipsSimpleEnglishRulesWhenDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.SimpleEnglish.Enabled = false
	report := Check("Keep retries available\n\nWorkers shouldn't utilize stale state.", cfg)
	if strings.Contains(findingCodes(report.Findings), "PC015") || strings.Contains(findingCodes(report.Findings), "PC016") {
		t.Fatalf("expected the Simple English profile to stay off, got %#v", report.Findings)
	}
}

func TestCheckProtectsTechnicalAndAllowedText(t *testing.T) {
	cfg := config.Default()
	cfg.SimpleEnglish.Enabled = true
	cfg.SimpleEnglish.Allow = []string{"could"}
	message := "Keep error details\n\nThe log contains `can't connect`. OAuth could return through SAML in src/might.go."

	report := Check(message, cfg)
	for _, finding := range report.Findings {
		if finding.Code == "PC015" || finding.Code == "PC017" {
			t.Fatalf("expected protected text to pass, got %#v", report.Findings)
		}
	}
}

func TestSimpleEnglishSeverityAndRuleOverride(t *testing.T) {
	cfg := config.Default()
	cfg.SimpleEnglish.Enabled = true
	cfg.SimpleEnglish.Severity = "error"
	cfg.Rules["PC015"] = "info"

	report := Check("Keep retries available\n\nWorkers shouldn't utilize stale state.", cfg)
	for _, finding := range report.Findings {
		switch finding.Code {
		case "PC015":
			if finding.Severity != SeverityInfo {
				t.Fatalf("expected the rule override, got %s", finding.Severity)
			}
		case "PC016":
			if finding.Severity != SeverityError {
				t.Fatalf("expected the profile severity, got %s", finding.Severity)
			}
		}
	}
}

func TestPragmaticSimpleEnglishKeepsContextRulesAsNotes(t *testing.T) {
	cfg := config.Default()
	report := Check("Keep retry state\n\nThe worker might lose records that have been saved.", cfg)

	found := 0
	for _, finding := range report.Findings {
		if finding.Code == "PC017" || finding.Code == "PC019" {
			found++
			if finding.Severity != SeverityInfo {
				t.Fatalf("expected %s to be a note, got %s", finding.Code, finding.Severity)
			}
		}
	}
	if found != 2 {
		t.Fatalf("expected two context findings, got %#v", report.Findings)
	}
}

func TestStrictSimpleEnglishEnforcesContextRules(t *testing.T) {
	cfg := config.Default()
	cfg.SimpleEnglish.Mode = config.SimpleEnglishStrict
	report := Check("Keep retry state\n\nThe worker might lose records that have been saved.", cfg)

	found := 0
	for _, finding := range report.Findings {
		if finding.Code == "PC017" || finding.Code == "PC019" {
			found++
			if finding.Severity != SeverityWarning {
				t.Fatalf("expected %s to be a warning, got %s", finding.Code, finding.Severity)
			}
		}
	}
	if found != 2 {
		t.Fatalf("expected two context findings, got %#v", report.Findings)
	}
}

func TestParseIgnoresGitComments(t *testing.T) {
	message := Parse("\nKeep cache keys stable\n\nThe old keys remain readable.\n# Please enter the commit message\n")
	if message.Subject != "Keep cache keys stable" {
		t.Fatalf("unexpected subject %q", message.Subject)
	}
	if strings.Contains(message.Body, "Please enter") {
		t.Fatalf("expected Git comment to be ignored, got %q", message.Body)
	}
}

func findingCodes(findings []Finding) string {
	codes := make([]string, 0, len(findings))
	for _, finding := range findings {
		codes = append(codes, finding.Code)
	}
	return strings.Join(codes, ",")
}
