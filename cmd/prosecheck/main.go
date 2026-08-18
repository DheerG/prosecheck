package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dheer/prosecheck/internal/checker"
	"github.com/dheer/prosecheck/internal/config"
	"github.com/dheer/prosecheck/internal/gitutil"
	"github.com/dheer/prosecheck/internal/hook"
	"github.com/dheer/prosecheck/internal/semantic"
)

const version = "0.1.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdin, stdout, stderr)
	case "install-hook":
		return runInstallHook(args[1:], stdout, stderr)
	case "uninstall-hook":
		return runUninstallHook(args[1:], stdout, stderr)
	case "version", "--version", "-v":
		fmt.Fprintln(stdout, version)
		return 0
	case "help", "--help", "-h":
		writeUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown command %q.\n\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runCheck(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	messageText := fs.String("message", "", "check this message instead of a file")
	configPath := fs.String("config", "", "read configuration from this file")
	format := fs.String("format", "text", "output format: text or json")
	semanticMode := fs.String("semantic", "auto", "local model use: auto, on, or off")
	strict := fs.Bool("strict", false, "return an error for warnings")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "The check command accepts no more than one message file.")
		return 2
	}
	if *messageText != "" && fs.NArg() == 1 {
		fmt.Fprintln(stderr, "Use --message or a message file, not both.")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(stderr, "The --format value must be text or json.")
		return 2
	}
	if *semanticMode != "auto" && *semanticMode != "on" && *semanticMode != "off" {
		fmt.Fprintln(stderr, "The --semantic value must be auto, on, or off.")
		return 2
	}

	message, err := readMessage(*messageText, fs.Args(), stdin)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the commit message: %v\n", err)
		return 2
	}

	cfg, loadedPath, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the configuration: %v\n", err)
		return 2
	}

	report := checker.Check(message, cfg)
	report.ConfigPath = loadedPath

	useSemantic := *semanticMode == "on" || (*semanticMode == "auto" && cfg.Semantic.Enabled)
	if useSemantic {
		if strings.TrimSpace(cfg.Semantic.Model) == "" {
			if *semanticMode == "on" {
				fmt.Fprintln(stderr, "The semantic model is not set. Set semantic.model in .prosecheck.json.")
				return 2
			}
			report.Findings = append(report.Findings, checker.Finding{
				Code:       "PC900",
				Severity:   checker.SeverityInfo,
				Source:     checker.SourceSystem,
				Message:    "The semantic review is enabled, but no model is set.",
				Suggestion: "Set semantic.model, or turn off the semantic review.",
			})
		} else {
			timeout, parseErr := time.ParseDuration(cfg.Semantic.Timeout)
			if parseErr != nil {
				fmt.Fprintf(stderr, "The semantic timeout is not valid: %v\n", parseErr)
				return 2
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			diff, _ := gitutil.StagedDiff(ctx, cfg.Semantic.MaxDiffBytes)
			client := semantic.NewClient(semantic.Options{
				Endpoint: cfg.Semantic.Endpoint,
				Model:    cfg.Semantic.Model,
				Timeout:  timeout,
			})
			findings, reviewErr := client.Review(ctx, message, diff)
			if reviewErr != nil {
				if *semanticMode == "on" {
					fmt.Fprintf(stderr, "The semantic review failed: %v\n", reviewErr)
					return 2
				}
				report.Findings = append(report.Findings, checker.Finding{
					Code:       "PC901",
					Severity:   checker.SeverityInfo,
					Source:     checker.SourceSystem,
					Message:    "The semantic review did not run.",
					Suggestion: reviewErr.Error(),
				})
			} else {
				report.SemanticUsed = true
				report.Findings = append(report.Findings, findings...)
			}
		}
	}

	checker.SortFindings(report.Findings)
	if *format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "Cannot write the report: %v\n", err)
			return 2
		}
	} else {
		writeTextReport(stdout, report)
	}

	if report.Failed(*strict) {
		return 1
	}
	return 0
}

func readMessage(flagValue string, args []string, stdin io.Reader) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if len(args) == 1 {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", errors.New("provide --message, a message file, or standard input")
	}
	return string(data), nil
}

func writeTextReport(w io.Writer, report checker.Report) {
	if len(report.Findings) == 0 {
		fmt.Fprintln(w, "Commit message passed.")
		return
	}
	for _, finding := range report.Findings {
		location := ""
		if finding.Line > 0 {
			location = fmt.Sprintf(" line %d", finding.Line)
		}
		fmt.Fprintf(w, "%s %s%s: %s\n", strings.ToUpper(string(finding.Severity)), finding.Code, location, finding.Message)
		if finding.Suggestion != "" {
			fmt.Fprintf(w, "  %s\n", finding.Suggestion)
		}
	}

	errorsCount, warningsCount, infoCount := report.Counts()
	parts := make([]string, 0, 3)
	if errorsCount > 0 {
		parts = append(parts, countLabel(errorsCount, "error"))
	}
	if warningsCount > 0 {
		parts = append(parts, countLabel(warningsCount, "warning"))
	}
	if infoCount > 0 {
		parts = append(parts, countLabel(infoCount, "note"))
	}
	fmt.Fprintln(w, strings.Join(parts, ", "))
}

func countLabel(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

func runInstallHook(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install-hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "The install-hook command does not accept arguments.")
		return 2
	}
	path, err := hook.Install("")
	if err != nil {
		fmt.Fprintf(stderr, "Cannot install the hook: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "Installed the commit-msg hook at %s.\n", path)
	return 0
}

func runUninstallHook(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall-hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "The uninstall-hook command does not accept arguments.")
		return 2
	}
	path, err := hook.Uninstall("")
	if err != nil {
		fmt.Fprintf(stderr, "Cannot remove the hook: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "Removed the commit-msg hook at %s.\n", path)
	return 0
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, `prosecheck checks Git commit messages.

Usage:
  prosecheck check [flags] [message-file]
  prosecheck install-hook
  prosecheck uninstall-hook
  prosecheck version

Examples:
  prosecheck check --message "Explain why sessions now expire"
  prosecheck check .git/COMMIT_EDITMSG
  git log -1 --format=%B | prosecheck check

Run "prosecheck check -h" to list the check flags.`)
}
