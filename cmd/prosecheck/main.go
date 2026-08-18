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

	"github.com/DheerG/prosecheck/internal/checker"
	"github.com/DheerG/prosecheck/internal/config"
	"github.com/DheerG/prosecheck/internal/gitutil"
	"github.com/DheerG/prosecheck/internal/hook"
	"github.com/DheerG/prosecheck/internal/modelruntime"
	"github.com/DheerG/prosecheck/internal/semantic"
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
	case "init":
		return runInit(args[1:], stdin, stdout, stderr)
	case "install-hook":
		return runInstallHook(args[1:], stdout, stderr)
	case "uninstall-hook":
		return runUninstallHook(args[1:], stdout, stderr)
	case "model":
		return runModel(args[1:], stdout, stderr)
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
	strict := fs.Bool("strict", true, "return an error for warnings; use --strict=false to allow them")
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
	if os.Getenv("PROSECHECK_BYPASS") == "1" {
		fmt.Fprintln(stderr, "Prosecheck skipped because PROSECHECK_BYPASS=1.")
		return 0
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
			endpoint := cfg.Semantic.Endpoint
			model := cfg.Semantic.Model
			if cfg.Semantic.Runtime == "managed" {
				manager, managerErr := modelruntime.New()
				if managerErr == nil {
					startContext, cancelStart := context.WithTimeout(context.Background(), 2*time.Minute)
					var state modelruntime.State
					state, managerErr = manager.EnsureRunning(startContext)
					cancelStart()
					if managerErr == nil {
						endpoint = state.Endpoint
						model = state.Model
					}
				}
				if managerErr != nil {
					if *semanticMode == "on" {
						fmt.Fprintf(stderr, "The semantic review failed: %v\n", managerErr)
						return 2
					}
					report.Findings = append(report.Findings, checker.Finding{
						Code:       "PC901",
						Severity:   checker.SeverityInfo,
						Source:     checker.SourceSystem,
						Message:    "The semantic review did not run.",
						Suggestion: managerErr.Error(),
					})
					useSemantic = false
				}
			}
			if !useSemantic {
				checker.SortFindings(report.Findings)
				return writeReport(stdout, stderr, report, *format, *strict)
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			diff, _ := gitutil.StagedDiff(ctx, cfg.Semantic.MaxDiffBytes)
			client := semantic.NewClient(semantic.Options{
				Endpoint: endpoint,
				Model:    model,
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
	return writeReport(stdout, stderr, report, *format, *strict)
}

func writeReport(stdout, stderr io.Writer, report checker.Report, format string, strict bool) int {
	if format == "json" {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "Cannot write the report: %v\n", err)
			return 2
		}
	} else {
		writeTextReport(stdout, report)
	}

	if report.Failed(strict) {
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

func runModel(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeModelUsage(stderr)
		return 2
	}
	manager, err := modelruntime.New()
	if err != nil {
		fmt.Fprintf(stderr, "Cannot prepare the model manager: %v\n", err)
		return 2
	}

	switch args[0] {
	case "install":
		return runModelInstall(manager, args[1:], stdout, stderr)
	case "start":
		return runModelStart(manager, args[1:], stdout, stderr)
	case "status":
		return runModelStatus(manager, args[1:], stdout, stderr)
	case "doctor":
		return runModelDoctor(manager, args[1:], stdout, stderr)
	case "logs":
		return runModelLogs(manager, args[1:], stdout, stderr)
	case "stop":
		return runModelStop(manager, args[1:], stdout, stderr)
	case "help", "--help", "-h":
		writeModelUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown model command %q.\n\n", args[0])
		writeModelUsage(stderr)
		return 2
	}
}

func runModelInstall(manager *modelruntime.Manager, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("model install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	modelFile := fs.String("model-file", "", "import this model file instead of downloading it")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 1 || (fs.NArg() == 1 && fs.Arg(0) != modelruntime.ModelName) {
		fmt.Fprintf(stderr, "The supported model is %s.\n", modelruntime.ModelName)
		return 2
	}
	installContext, cancelInstall := modelInstallContext()
	defer cancelInstall()
	progress := newInstallProgressWriter(stdout)
	defer progress.Finish()
	if err := manager.Install(installContext, *modelFile, progress.Report); err != nil {
		progress.Finish()
		fmt.Fprintf(stderr, "Cannot install the model: %v\n", err)
		return 2
	}
	return 0
}

func runModelStart(manager *modelruntime.Manager, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("model start", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", 0, "preferred local port; default 11435")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *port < 0 || *port > 65535 {
		fmt.Fprintln(stderr, "The start command accepts an optional port from 1 to 65535.")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	state, err := manager.Start(ctx, *port)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot start the model: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "Ministral is ready at %s.\n", state.Endpoint)
	return 0
}

func runModelStatus(manager *modelruntime.Manager, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "The status command does not accept arguments.")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	status := manager.Status(ctx)
	fmt.Fprintf(stdout, "Model file: %s\n", status.ModelPath)
	if !status.Installed {
		fmt.Fprintln(stdout, "Ministral is not installed.")
		fmt.Fprintf(stdout, "Run `prosecheck model install %s`.\n", modelruntime.ModelName)
		return 1
	}
	if !status.Running {
		fmt.Fprintln(stdout, "Ministral is installed but stopped.")
		return 1
	}
	fmt.Fprintf(stdout, "Ministral is running at %s.\n", status.State.Endpoint)
	return 0
}

func runModelDoctor(manager *modelruntime.Manager, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "The doctor command does not accept arguments.")
		return 2
	}
	startContext, cancelStart := context.WithTimeout(context.Background(), 2*time.Minute)
	state, err := manager.EnsureRunning(startContext)
	cancelStart()
	if err != nil {
		fmt.Fprintf(stderr, "The model is not ready: %v\n", err)
		return 2
	}
	reviewContext, cancelReview := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelReview()
	client := semantic.NewClient(semantic.Options{
		Endpoint: state.Endpoint, Model: state.Model, Timeout: 30 * time.Second,
	})
	_, err = client.Review(reviewContext,
		"Prevent duplicate invoice delivery\n\nReject a repeated delivery before the queue accepts it.", "")
	if err != nil {
		fmt.Fprintf(stderr, "The model server started but could not complete a review: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "Ministral completed a test review at %s.\n", state.Endpoint)
	return 0
}

func runModelLogs(manager *modelruntime.Manager, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("model logs", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lines := fs.Int("lines", 40, "number of recent log lines")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 || *lines < 1 {
		fmt.Fprintln(stderr, "The lines value must be more than zero.")
		return 2
	}
	logs, err := manager.TailLogs(*lines)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the model log: %v\n", err)
		return 2
	}
	if logs == "" {
		fmt.Fprintf(stdout, "The model log is empty: %s\n", manager.Paths().Log)
		return 0
	}
	fmt.Fprintln(stdout, logs)
	return 0
}

func runModelStop(manager *modelruntime.Manager, args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "The stop command does not accept arguments.")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := manager.Stop(ctx); err != nil {
		fmt.Fprintf(stderr, "Cannot stop the model: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, "Ministral is stopped.")
	return 0
}

func writeModelUsage(w io.Writer) {
	fmt.Fprintln(w, `prosecheck manages a private Ministral model server.

Usage:
  prosecheck model install [--model-file path] [ministral-3-8b]
  prosecheck model start [--port number]
  prosecheck model status
  prosecheck model doctor
  prosecheck model logs [--lines number]
  prosecheck model stop`)
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, `prosecheck checks Git commit messages.

Usage:
  prosecheck check [flags] [message-file]
  prosecheck init
  prosecheck install-hook
  prosecheck uninstall-hook
  prosecheck model <command>
  prosecheck version

Examples:
  prosecheck check --message "Explain why sessions now expire"
  prosecheck check .git/COMMIT_EDITMSG
  git log -1 --format=%B | prosecheck check

Run "prosecheck check -h" to list the check flags.
Run "prosecheck init -h" to list the setup flags.`)
}
