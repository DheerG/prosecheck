package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	proseeval "github.com/DheerG/prosecheck/internal/eval"
	"github.com/DheerG/prosecheck/internal/semantic"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prosecheck-eval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	endpoint := fs.String("endpoint", "", "OpenAI-compatible endpoint")
	model := fs.String("model", "", "model name sent to the endpoint")
	casesPath := fs.String("cases", "evals/semantic.json", "path to the eval cases")
	repetitions := fs.Int("repeat", 1, "number of runs for each case")
	warmups := fs.Int("warmup", 1, "number of warmup requests")
	timeoutText := fs.String("timeout", "30s", "timeout for one model request")
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "The eval command does not accept positional arguments.")
		return 2
	}
	if *endpoint == "" || *model == "" {
		fmt.Fprintln(stderr, "Set both --endpoint and --model.")
		return 2
	}
	if *repetitions < 1 || *warmups < 0 {
		fmt.Fprintln(stderr, "The repeat value must be positive. The warmup value cannot be negative.")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(stderr, "The format value must be text or json.")
		return 2
	}
	timeout, err := time.ParseDuration(*timeoutText)
	if err != nil || timeout <= 0 {
		fmt.Fprintln(stderr, "The timeout must be a positive Go duration, such as 30s.")
		return 2
	}

	suite, err := proseeval.Load(*casesPath)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the eval cases: %v\n", err)
		return 2
	}
	client := semantic.NewClient(semantic.Options{Endpoint: *endpoint, Model: *model, Timeout: timeout})
	requestCount := len(suite.Cases)*(*repetitions) + *warmups
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(requestCount)*timeout)
	defer cancel()
	for index := 0; index < *warmups; index++ {
		_, _ = client.Review(ctx, suite.Cases[0].Message, suite.Cases[0].Diff)
	}
	report := proseeval.Run(ctx, suite, client, proseeval.Options{Model: *model, Repetitions: *repetitions})

	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "Cannot write the eval report: %v\n", err)
			return 2
		}
		return 0
	}
	writeTextReport(stdout, report)
	return 0
}

func writeTextReport(w io.Writer, report proseeval.Report) {
	fmt.Fprintf(w, "Model: %s\n", report.Model)
	fmt.Fprintf(w, "Cases: %d (%d requests; %d runs per case)\n",
		report.CaseCount, report.RequestCount, report.Repetitions)
	fmt.Fprintf(w, "Precision: %.1f%%\n", report.Precision*100)
	fmt.Fprintf(w, "Recall: %.1f%%\n", report.Recall*100)
	fmt.Fprintf(w, "F1: %.1f%%\n", report.F1*100)
	fmt.Fprintf(w, "Exact matches: %.1f%%\n", report.ExactMatchRate*100)
	fmt.Fprintf(w, "Consistent cases: %.1f%%\n", report.ConsistencyRate*100)
	fmt.Fprintf(w, "Clear-case false positives: %.1f%%\n", report.ClearFalsePositiveRate*100)
	fmt.Fprintf(w, "Protocol errors: %d\n", report.ProtocolErrors)
	fmt.Fprintf(w, "Latency: %.0f ms mean, %.0f ms p50, %.0f ms p95\n",
		report.MeanLatencyMS, report.P50LatencyMS, report.P95LatencyMS)
	fmt.Fprintln(w, "Rule F1:")
	for number := 1; number <= 6; number++ {
		code := fmt.Sprintf("SEM%03d", number)
		score := report.Codes[code]
		fmt.Fprintf(w, "  %s: %.1f%% precision, %.1f%% recall, %.1f%% F1\n",
			code, score.Precision*100, score.Recall*100, score.F1*100)
	}

	type mismatchGroup struct {
		result proseeval.CaseResult
		count  int
	}
	groups := make([]mismatchGroup, 0)
	groupIndexes := make(map[string]int)
	for _, result := range report.Results {
		if result.Exact {
			continue
		}
		key := result.ID + "|" + strings.Join(result.ExpectedCodes, ",") + "|" +
			strings.Join(result.ActualCodes, ",") + "|" + result.Error
		if index, exists := groupIndexes[key]; exists {
			groups[index].count++
			continue
		}
		groupIndexes[key] = len(groups)
		groups = append(groups, mismatchGroup{result: result, count: 1})
	}
	if len(groups) > 0 {
		fmt.Fprintln(w, "Mismatches:")
	}
	for _, group := range groups {
		result := group.result
		frequency := ""
		if report.Repetitions > 1 {
			frequency = fmt.Sprintf(" (%d/%d runs)", group.count, report.Repetitions)
		}
		if result.Error != "" {
			fmt.Fprintf(w, "  %s%s: expected %v; error: %s\n",
				result.ID, frequency, result.ExpectedCodes, result.Error)
			continue
		}
		fmt.Fprintf(w, "  %s%s: expected %v; got %v\n",
			result.ID, frequency, result.ExpectedCodes, result.ActualCodes)
	}
}
