package checker

import "sort"

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

type Source string

const (
	SourceRule     Source = "rule"
	SourceSemantic Source = "semantic"
	SourceSystem   Source = "system"
)

type Finding struct {
	Code       string   `json:"code"`
	Severity   Severity `json:"severity"`
	Source     Source   `json:"source"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion,omitempty"`
	Line       int      `json:"line,omitempty"`
}

type Report struct {
	Subject      string    `json:"subject"`
	Findings     []Finding `json:"findings"`
	SemanticUsed bool      `json:"semanticUsed"`
	ConfigPath   string    `json:"configPath,omitempty"`
}

func (r Report) Counts() (errorsCount, warningsCount, infoCount int) {
	for _, finding := range r.Findings {
		switch finding.Severity {
		case SeverityError:
			errorsCount++
		case SeverityWarning:
			warningsCount++
		case SeverityInfo:
			infoCount++
		}
	}
	return errorsCount, warningsCount, infoCount
}

func (r Report) Failed(strict bool) bool {
	for _, finding := range r.Findings {
		if finding.Severity == SeverityError || (strict && finding.Severity == SeverityWarning) {
			return true
		}
	}
	return false
}

func SortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Code < findings[j].Code
	})
}
