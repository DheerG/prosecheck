package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/DheerG/prosecheck/internal/checker"
)

type Suite struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
	Cases       []Case `json:"cases"`
}

type Case struct {
	ID            string   `json:"id"`
	Category      string   `json:"category"`
	Message       string   `json:"message"`
	Diff          string   `json:"diff,omitempty"`
	ExpectedCodes []string `json:"expectedCodes"`
}

type Reviewer interface {
	Review(ctx context.Context, message, diff string) ([]checker.Finding, error)
}

type Options struct {
	Model       string
	Repetitions int
}

type Report struct {
	Model                  string                `json:"model"`
	SuiteVersion           int                   `json:"suiteVersion"`
	CaseCount              int                   `json:"caseCount"`
	Repetitions            int                   `json:"repetitions"`
	RequestCount           int                   `json:"requestCount"`
	TruePositives          int                   `json:"truePositives"`
	FalsePositives         int                   `json:"falsePositives"`
	FalseNegatives         int                   `json:"falseNegatives"`
	ExactMatches           int                   `json:"exactMatches"`
	ClearCaseCount         int                   `json:"clearCaseCount"`
	ClearFalsePositives    int                   `json:"clearFalsePositives"`
	ProtocolErrors         int                   `json:"protocolErrors"`
	Precision              float64               `json:"precision"`
	Recall                 float64               `json:"recall"`
	F1                     float64               `json:"f1"`
	ExactMatchRate         float64               `json:"exactMatchRate"`
	ConsistentCases        int                   `json:"consistentCases"`
	ConsistencyRate        float64               `json:"consistencyRate"`
	ClearFalsePositiveRate float64               `json:"clearFalsePositiveRate"`
	MeanLatencyMS          float64               `json:"meanLatencyMs"`
	P50LatencyMS           float64               `json:"p50LatencyMs"`
	P95LatencyMS           float64               `json:"p95LatencyMs"`
	Codes                  map[string]*CodeScore `json:"codes"`
	Results                []CaseResult          `json:"results"`
}

type CodeScore struct {
	TruePositives  int     `json:"truePositives"`
	FalsePositives int     `json:"falsePositives"`
	FalseNegatives int     `json:"falseNegatives"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"recall"`
	F1             float64 `json:"f1"`
}

type CaseResult struct {
	ID            string   `json:"id"`
	Category      string   `json:"category"`
	Repetition    int      `json:"repetition"`
	ExpectedCodes []string `json:"expectedCodes"`
	ActualCodes   []string `json:"actualCodes,omitempty"`
	Exact         bool     `json:"exact"`
	LatencyMS     float64  `json:"latencyMs"`
	Error         string   `json:"error,omitempty"`
}

func Load(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, err
	}
	var suite Suite
	if err := json.Unmarshal(data, &suite); err != nil {
		return Suite{}, err
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, err
	}
	return suite, nil
}

func (s Suite) Validate() error {
	if s.Version < 1 {
		return errors.New("the eval version must be more than zero")
	}
	if len(s.Cases) == 0 {
		return errors.New("the eval suite must contain at least one case")
	}
	seen := make(map[string]bool, len(s.Cases))
	for index, item := range s.Cases {
		if item.ID == "" || item.Category == "" || item.Message == "" {
			return fmt.Errorf("eval case %d must have an ID, category, and message", index+1)
		}
		if seen[item.ID] {
			return fmt.Errorf("eval case ID %q occurs more than once", item.ID)
		}
		seen[item.ID] = true
		for _, code := range item.ExpectedCodes {
			if code < "SEM001" || code > "SEM006" {
				return fmt.Errorf("eval case %q has unknown code %q", item.ID, code)
			}
		}
	}
	return nil
}

func Run(ctx context.Context, suite Suite, reviewer Reviewer, options Options) Report {
	repetitions := options.Repetitions
	if repetitions < 1 {
		repetitions = 1
	}
	report := Report{
		Model: options.Model, SuiteVersion: suite.Version, CaseCount: len(suite.Cases), Repetitions: repetitions,
		RequestCount: len(suite.Cases) * repetitions,
		Codes:        make(map[string]*CodeScore, 6),
		Results:      make([]CaseResult, 0, len(suite.Cases)*repetitions),
	}
	for number := 1; number <= 6; number++ {
		report.Codes[fmt.Sprintf("SEM%03d", number)] = &CodeScore{}
	}
	latencies := make([]float64, 0, report.RequestCount)

	for repetition := 1; repetition <= repetitions; repetition++ {
		for _, item := range suite.Cases {
			expected := sortedUnique(item.ExpectedCodes)
			if len(expected) == 0 {
				report.ClearCaseCount++
			}
			started := time.Now()
			findings, err := reviewer.Review(ctx, item.Message, item.Diff)
			latency := float64(time.Since(started).Microseconds()) / 1000
			latencies = append(latencies, latency)
			result := CaseResult{
				ID: item.ID, Category: item.Category, Repetition: repetition,
				ExpectedCodes: expected, LatencyMS: latency,
			}
			if err != nil {
				result.Error = err.Error()
				report.ProtocolErrors++
				report.FalseNegatives += len(result.ExpectedCodes)
				for _, code := range result.ExpectedCodes {
					report.Codes[code].FalseNegatives++
				}
				report.Results = append(report.Results, result)
				continue
			}
			actual := make([]string, 0, len(findings))
			for _, finding := range findings {
				actual = append(actual, finding.Code)
			}
			result.ActualCodes = sortedUnique(actual)
			result.Exact = equalCodes(result.ExpectedCodes, result.ActualCodes)
			if result.Exact {
				report.ExactMatches++
			}
			truePositive, falsePositive, falseNegative := compareCodes(result.ExpectedCodes, result.ActualCodes)
			report.TruePositives += truePositive
			report.FalsePositives += falsePositive
			report.FalseNegatives += falseNegative
			updateCodeScores(report.Codes, result.ExpectedCodes, result.ActualCodes)
			if len(result.ExpectedCodes) == 0 && len(result.ActualCodes) > 0 {
				report.ClearFalsePositives++
			}
			report.Results = append(report.Results, result)
		}
	}

	report.Precision = ratio(report.TruePositives, report.TruePositives+report.FalsePositives)
	report.Recall = ratio(report.TruePositives, report.TruePositives+report.FalseNegatives)
	if report.Precision+report.Recall > 0 {
		report.F1 = 2 * report.Precision * report.Recall / (report.Precision + report.Recall)
	}
	report.ExactMatchRate = ratio(report.ExactMatches, report.RequestCount)
	report.ConsistentCases = consistentCases(report.Results)
	report.ConsistencyRate = ratio(report.ConsistentCases, report.CaseCount)
	report.ClearFalsePositiveRate = ratio(report.ClearFalsePositives, report.ClearCaseCount)
	for _, score := range report.Codes {
		score.Precision = ratio(score.TruePositives, score.TruePositives+score.FalsePositives)
		score.Recall = ratio(score.TruePositives, score.TruePositives+score.FalseNegatives)
		if score.Precision+score.Recall > 0 {
			score.F1 = 2 * score.Precision * score.Recall / (score.Precision + score.Recall)
		}
	}
	report.MeanLatencyMS = mean(latencies)
	report.P50LatencyMS = percentile(latencies, 0.50)
	report.P95LatencyMS = percentile(latencies, 0.95)
	return report
}

func consistentCases(results []CaseResult) int {
	outcomes := make(map[string]string)
	consistent := make(map[string]bool)
	for _, result := range results {
		outcome := fmt.Sprintf("%v|%s", result.ActualCodes, result.Error)
		previous, exists := outcomes[result.ID]
		if !exists {
			outcomes[result.ID] = outcome
			consistent[result.ID] = true
			continue
		}
		if previous != outcome {
			consistent[result.ID] = false
		}
	}
	count := 0
	for _, matches := range consistent {
		if matches {
			count++
		}
	}
	return count
}

func updateCodeScores(scores map[string]*CodeScore, expected, actual []string) {
	expectedSet := make(map[string]bool, len(expected))
	actualSet := make(map[string]bool, len(actual))
	for _, code := range expected {
		expectedSet[code] = true
	}
	for _, code := range actual {
		actualSet[code] = true
	}
	for code, score := range scores {
		switch {
		case expectedSet[code] && actualSet[code]:
			score.TruePositives++
		case expectedSet[code]:
			score.FalseNegatives++
		case actualSet[code]:
			score.FalsePositives++
		}
	}
}

func compareCodes(expected, actual []string) (truePositive, falsePositive, falseNegative int) {
	expectedSet := make(map[string]bool, len(expected))
	actualSet := make(map[string]bool, len(actual))
	for _, code := range expected {
		expectedSet[code] = true
	}
	for _, code := range actual {
		actualSet[code] = true
		if expectedSet[code] {
			truePositive++
		} else {
			falsePositive++
		}
	}
	for _, code := range expected {
		if !actualSet[code] {
			falseNegative++
		}
	}
	return truePositive, falsePositive, falseNegative
}

func sortedUnique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func equalCodes(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func percentile(values []float64, quantile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(float64(len(sorted)-1) * quantile)
	return sorted[index]
}
