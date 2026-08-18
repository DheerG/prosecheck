package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DheerG/prosecheck/internal/config"
	"github.com/DheerG/prosecheck/internal/gitutil"
	"github.com/DheerG/prosecheck/internal/hook"
	"github.com/DheerG/prosecheck/internal/modelruntime"
)

type initChoices struct {
	simpleEnglishMode string
	semantic          bool
	hook              bool
}

type answerReader struct {
	scanner *bufio.Scanner
	output  io.Writer
}

func runInit(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	yes := fs.Bool("yes", false, "use recommended choices without questions")
	advanced := fs.Bool("advanced", false, "choose each feature")
	repository := fs.String("repository", "", "set up this Git repository")
	simpleEnglish := fs.String("simple-english", "", "Simple English: pragmatic, strict, or off")
	semantic := fs.String("semantic", "", "private AI review on this computer: on or off")
	installHook := fs.String("hook", "", "Git commit hook: on or off")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "The init command does not accept arguments.")
		return 2
	}
	if *yes && *advanced {
		fmt.Fprintln(stderr, "Use --yes or --advanced, not both.")
		return 2
	}
	if !validSimpleEnglishMode(*simpleEnglish) {
		fmt.Fprintln(stderr, "--simple-english must be pragmatic, strict, or off.")
		return 2
	}
	for name, value := range map[string]string{
		"--semantic": *semantic,
		"--hook":     *installHook,
	} {
		if !validToggle(value) {
			fmt.Fprintf(stderr, "%s must be on or off.\n", name)
			return 2
		}
	}

	root, err := gitutil.Root(*repository)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot start setup: %v.\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "Found the Git repository at %s.\n", root)

	reader := answerReader{scanner: bufio.NewScanner(stdin), output: stdout}
	choices, err := chooseInitOptions(reader, *yes, *advanced, *simpleEnglish, *semantic, *installHook)
	if err != nil {
		fmt.Fprintf(stderr, "Cannot read the setup choice: %v\n", err)
		return 2
	}
	writeInitSummary(stdout, choices)
	if !*yes {
		proceed, askErr := reader.ask("Continue with this setup?", true)
		if askErr != nil {
			fmt.Fprintf(stderr, "Cannot read the setup choice: %v\n", askErr)
			return 2
		}
		if !proceed {
			fmt.Fprintln(stdout, "Setup canceled. No files changed.")
			return 0
		}
	}

	configPath := filepath.Join(root, config.FileName)
	cfg := config.Default()
	writeConfig := true
	if _, statErr := os.Stat(configPath); statErr == nil {
		fmt.Fprintf(stdout, "Found %s.\n", configPath)
		cfg, _, err = config.Load(configPath)
		if err != nil {
			fmt.Fprintf(stderr, "Cannot read the existing configuration: %v\n", err)
			return 2
		}
		if !*yes {
			writeConfig, err = reader.ask("Update the existing configuration?", true)
			if err != nil {
				fmt.Fprintf(stderr, "Cannot read the setup choice: %v\n", err)
				return 2
			}
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		fmt.Fprintf(stderr, "Cannot inspect the configuration: %v\n", statErr)
		return 2
	}

	if !writeConfig {
		fmt.Fprintln(stdout, "Kept the existing configuration.")
	}

	if choices.semantic {
		manager, managerErr := modelruntime.New()
		if managerErr != nil {
			fmt.Fprintf(stderr, "Cannot prepare the local model: %v\n", managerErr)
			return 2
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		installed := manager.Status(ctx).Installed
		cancel()
		if !installed {
			fmt.Fprintln(stdout, "Installing the private AI reviewer.")
			if installErr := manager.Install(context.Background(), "", func(message string) {
				fmt.Fprintln(stdout, message)
			}); installErr != nil {
				fmt.Fprintf(stderr, "Cannot install the local model: %v\n", installErr)
				return 2
			}
		} else {
			fmt.Fprintln(stdout, "The private AI reviewer is already installed.")
		}
	}

	if writeConfig {
		cfg.SimpleEnglish.Enabled = choices.simpleEnglishMode != "off"
		if cfg.SimpleEnglish.Enabled {
			cfg.SimpleEnglish.Mode = choices.simpleEnglishMode
		}
		cfg.Semantic.Enabled = choices.semantic
		if choices.semantic {
			cfg.Semantic.Runtime = "managed"
			cfg.Semantic.Endpoint = ""
			cfg.Semantic.Model = modelruntime.ModelName
		}
		if err := config.Write(configPath, cfg); err != nil {
			fmt.Fprintf(stderr, "Cannot write the configuration: %v\n", err)
			return 2
		}
		fmt.Fprintf(stdout, "Wrote %s.\n", configPath)
	}

	if choices.hook {
		path, installErr := hook.Install(root)
		if installErr != nil {
			var integrationErr *hook.IntegrationError
			if errors.As(installErr, &integrationErr) {
				fmt.Fprintln(stdout, "The hook manager needs one manual step.")
				fmt.Fprintln(stdout, integrationErr.Error())
				printInitFinish(stdout, configPath, writeConfig, false)
				return 1
			}
			fmt.Fprintf(stderr, "Cannot install the commit hook: %v\n", installErr)
			return 2
		}
		fmt.Fprintf(stdout, "Installed the commit hook at %s.\n", path)
	} else {
		fmt.Fprintln(stdout, "Skipped the commit hook.")
	}

	printInitFinish(stdout, configPath, writeConfig, true)
	return 0
}

func chooseInitOptions(reader answerReader, yes, advanced bool, simpleValue, semanticValue, hookValue string) (initChoices, error) {
	choices := initChoices{simpleEnglishMode: config.SimpleEnglishPragmatic, semantic: false, hook: true}
	if yes {
		applySimpleEnglishMode(&choices.simpleEnglishMode, simpleValue)
		applyToggle(&choices.semantic, semanticValue)
		applyToggle(&choices.hook, hookValue)
		return choices, nil
	}

	if !advanced && simpleValue == "" && hookValue == "" {
		fmt.Fprintln(reader.output, "The recommended setup checks each commit and uses pragmatic Simple English.")
		fmt.Fprintln(reader.output, "A built-in rule can stop a commit until you correct its message.")
		recommended, err := reader.ask("Use the recommended local rules and Git hook?", true)
		if err != nil {
			return choices, err
		}
		advanced = !recommended
	}
	if advanced {
		var err error
		if simpleValue == "" {
			fmt.Fprintln(reader.output, "Pragmatic mode blocks clear problems and reports uncertain grammar as notes.")
			enabled, askErr := reader.ask("Use Simple English?", true)
			if askErr != nil {
				return choices, askErr
			}
			if !enabled {
				choices.simpleEnglishMode = "off"
			} else {
				fmt.Fprintln(reader.output, "Strict mode also blocks modal verbs and complex verb tenses.")
				strict, strictErr := reader.ask("Use strict Simple English?", false)
				if strictErr != nil {
					return choices, strictErr
				}
				if strict {
					choices.simpleEnglishMode = config.SimpleEnglishStrict
				}
			}
		}
		if hookValue == "" {
			choices.hook, err = reader.ask("Install the Git commit hook?", true)
			if err != nil {
				return choices, err
			}
		}
	}
	applySimpleEnglishMode(&choices.simpleEnglishMode, simpleValue)
	applyToggle(&choices.hook, hookValue)

	if semanticValue == "" {
		fmt.Fprintln(reader.output, "The optional AI reviewer runs on this computer and requires a large download.")
		fmt.Fprintln(reader.output, "Its advice does not stop commits.")
		var err error
		choices.semantic, err = reader.ask("Install the private AI reviewer?", false)
		if err != nil {
			return choices, err
		}
	} else {
		applyToggle(&choices.semantic, semanticValue)
	}
	return choices, nil
}

func (r answerReader) ask(question string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	for {
		fmt.Fprintf(r.output, "%s %s ", question, suffix)
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return false, err
			}
			return false, errors.New("input ended before you selected an answer")
		}
		switch strings.ToLower(strings.TrimSpace(r.scanner.Text())) {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(r.output, "Enter y or n.")
		}
	}
}

func validToggle(value string) bool {
	return value == "" || value == "on" || value == "off"
}

func validSimpleEnglishMode(value string) bool {
	return value == "" || value == "off" ||
		value == config.SimpleEnglishPragmatic || value == config.SimpleEnglishStrict
}

func applySimpleEnglishMode(target *string, value string) {
	switch value {
	case config.SimpleEnglishPragmatic:
		*target = config.SimpleEnglishPragmatic
	case config.SimpleEnglishStrict:
		*target = config.SimpleEnglishStrict
	case "off":
		*target = "off"
	}
}

func applyToggle(target *bool, value string) {
	if value != "" {
		*target = value == "on"
	}
}

func writeInitSummary(output io.Writer, choices initChoices) {
	fmt.Fprintln(output, "Selected setup:")
	fmt.Fprintf(output, "  Simple English: %s\n", choices.simpleEnglishMode)
	fmt.Fprintf(output, "  Private AI review: %s\n", toggleLabel(choices.semantic))
	fmt.Fprintf(output, "  Git commit hook: %s\n", toggleLabel(choices.hook))
}

func toggleLabel(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func printInitFinish(output io.Writer, configPath string, configChanged, hookReady bool) {
	if hookReady {
		fmt.Fprintln(output, "Setup is complete.")
	} else {
		fmt.Fprintln(output, "The configuration is ready. Complete the hook step above.")
	}
	if configChanged {
		fmt.Fprintln(output, "Add the configuration to Git:")
		fmt.Fprintf(output, "  git add %s\n", filepath.Base(configPath))
	}
}
