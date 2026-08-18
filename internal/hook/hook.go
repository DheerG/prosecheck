package hook

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	managedFileMarker = "# Managed by prosecheck."
	blockStart        = "# prosecheck:start"
	blockEnd          = "# prosecheck:end"
)

const plainScript = `#!/bin/sh
# Managed by prosecheck.

if ! command -v prosecheck >/dev/null 2>&1; then
  echo "prosecheck is not available on PATH." >&2
  echo "Install prosecheck or remove this hook with: prosecheck uninstall-hook" >&2
  exit 1
fi

exec prosecheck check --strict "$1"
`

const managedBlock = `# prosecheck:start
if ! command -v prosecheck >/dev/null 2>&1; then
  echo "prosecheck is not available on PATH." >&2
  exit 1
fi
prosecheck check --strict "$1" || exit $?
# prosecheck:end
`

type Manager string

const (
	ManagerGit        Manager = "Git"
	ManagerHusky      Manager = "Husky"
	ManagerLefthook   Manager = "Lefthook"
	ManagerPreCommit  Manager = "pre-commit"
	ManagerOvercommit Manager = "Overcommit"
)

// IntegrationError explains how to add prosecheck to a hook manager that
// owns its configuration file. prosecheck does not edit these configuration
// formats because it cannot preserve comments and custom structure safely.
type IntegrationError struct {
	Manager    Manager
	ConfigPath string
	Remove     bool
}

func (e *IntegrationError) Error() string {
	if e.Remove {
		return removeInstructions(e.Manager, e.ConfigPath)
	}
	return installInstructions(e.Manager, e.ConfigPath)
}

func Install(repository string) (string, error) {
	root, err := repositoryRoot(repository)
	if err != nil {
		return "", err
	}
	path, err := hookPath(root)
	if err != nil {
		return "", err
	}
	manager, configPath, err := detectManager(root, path)
	if err != nil {
		return "", err
	}

	switch manager {
	case ManagerHusky:
		return installHusky(root)
	case ManagerLefthook, ManagerPreCommit, ManagerOvercommit:
		return "", &IntegrationError{Manager: manager, ConfigPath: configPath}
	default:
		return installPlain(path)
	}
}

func Uninstall(repository string) (string, error) {
	root, err := repositoryRoot(repository)
	if err != nil {
		return "", err
	}
	path, err := hookPath(root)
	if err != nil {
		return "", err
	}
	huskyPath := filepath.Join(root, ".husky", "commit-msg")
	if data, readErr := os.ReadFile(huskyPath); readErr == nil && strings.Contains(string(data), blockStart) {
		return uninstallHusky(root)
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	if data, readErr := os.ReadFile(path); readErr == nil && strings.Contains(string(data), managedFileMarker) {
		return uninstallPlain(path)
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	manager, configPath, err := detectManager(root, path)
	if err != nil {
		return "", err
	}

	switch manager {
	case ManagerHusky:
		return uninstallHusky(root)
	case ManagerLefthook, ManagerPreCommit, ManagerOvercommit:
		return "", &IntegrationError{Manager: manager, ConfigPath: configPath, Remove: true}
	default:
		return uninstallPlain(path)
	}
}

func installPlain(path string) (string, error) {
	if data, readErr := os.ReadFile(path); readErr == nil {
		if !strings.Contains(string(data), managedFileMarker) {
			return "", fmt.Errorf("%s already exists. Add `prosecheck check --strict \"$1\"` to that hook", path)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(plainScript), 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func uninstallPlain(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(data), managedFileMarker) {
		return "", fmt.Errorf("%s is not managed by prosecheck. Remove the prosecheck command from that hook", path)
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func installHusky(root string) (string, error) {
	path := filepath.Join(root, ".husky", "commit-msg")
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	hasStart := strings.Contains(string(data), blockStart)
	hasEnd := strings.Contains(string(data), blockEnd)
	if hasStart && hasEnd {
		return path, nil
	}
	if hasStart || hasEnd {
		return "", fmt.Errorf("the prosecheck block in %s is incomplete", path)
	}
	if strings.Contains(string(data), "prosecheck check") {
		return "", fmt.Errorf("%s already contains a prosecheck command that prosecheck does not manage", path)
	}

	content := string(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += managedBlock

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func uninstallHusky(root string) (string, error) {
	path := filepath.Join(root, ".husky", "commit-msg")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}

	content := string(data)
	start := strings.Index(content, blockStart)
	if start < 0 {
		return "", fmt.Errorf("%s does not contain a prosecheck block", path)
	}
	endOffset := strings.Index(content[start:], blockEnd)
	if endOffset < 0 {
		return "", fmt.Errorf("the prosecheck block in %s is incomplete", path)
	}
	end := start + endOffset + len(blockEnd)
	if end < len(content) && content[end] == '\n' {
		end++
	}

	remaining := content[:start] + content[end:]
	if strings.TrimSpace(remaining) == "" {
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return path, nil
	}
	if err := os.WriteFile(path, []byte(remaining), 0o755); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func repositoryRoot(repository string) (string, error) {
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	if repository != "" {
		command.Dir = repository
	}
	output, err := command.Output()
	if err != nil {
		return "", errors.New("the current directory is not a Git repository")
	}
	return filepath.Clean(strings.TrimSpace(string(output))), nil
}

func hookPath(repository string) (string, error) {
	command := exec.Command("git", "rev-parse", "--git-path", "hooks/commit-msg")
	command.Dir = repository
	output, err := command.Output()
	if err != nil {
		return "", errors.New("the current directory is not a Git repository")
	}
	path := strings.TrimSpace(string(output))
	if !filepath.IsAbs(path) {
		path = filepath.Join(repository, path)
	}
	return filepath.Clean(path), nil
}

func detectManager(root, path string) (Manager, string, error) {
	hooksPath, err := configuredHooksPath(root)
	if err != nil {
		return "", "", err
	}
	if isHuskyPath(hooksPath) {
		return ManagerHusky, filepath.Join(root, ".husky"), nil
	}

	if data, readErr := os.ReadFile(path); readErr == nil {
		if manager := managerFromHook(string(data)); manager != ManagerGit {
			return manager, managerConfigPath(root, manager), nil
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", "", readErr
	}

	if info, statErr := os.Stat(filepath.Join(root, ".husky")); statErr == nil && info.IsDir() {
		return ManagerHusky, filepath.Join(root, ".husky"), nil
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", "", statErr
	}

	for _, candidate := range managerConfigCandidates(root) {
		if info, statErr := os.Stat(candidate.path); statErr == nil && !info.IsDir() {
			return candidate.manager, candidate.path, nil
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", "", statErr
		}
	}
	return ManagerGit, "", nil
}

func configuredHooksPath(root string) (string, error) {
	command := exec.Command("git", "config", "--get", "core.hooksPath")
	command.Dir = root
	output, err := command.Output()
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", nil
	}
	return "", err
}

func isHuskyPath(path string) bool {
	clean := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(path)), "/")
	return clean == ".husky/_" || strings.HasSuffix(clean, "/.husky/_")
}

func managerFromHook(content string) Manager {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "generated by pre-commit") || strings.Contains(lower, "pre_commit"):
		return ManagerPreCommit
	case strings.Contains(lower, "lefthook"):
		return ManagerLefthook
	case strings.Contains(lower, "overcommit"):
		return ManagerOvercommit
	default:
		return ManagerGit
	}
}

type managerConfig struct {
	manager Manager
	path    string
}

func managerConfigCandidates(root string) []managerConfig {
	names := []struct {
		manager Manager
		name    string
	}{
		{ManagerLefthook, "lefthook.yml"},
		{ManagerLefthook, "lefthook.yaml"},
		{ManagerLefthook, ".lefthook.yml"},
		{ManagerLefthook, ".lefthook.yaml"},
		{ManagerLefthook, "lefthook.toml"},
		{ManagerLefthook, ".lefthook.toml"},
		{ManagerLefthook, "lefthook.json"},
		{ManagerLefthook, ".lefthook.json"},
		{ManagerLefthook, "lefthook.jsonc"},
		{ManagerLefthook, ".lefthook.jsonc"},
		{ManagerLefthook, filepath.Join(".config", "lefthook.yml")},
		{ManagerLefthook, filepath.Join(".config", "lefthook.yaml")},
		{ManagerLefthook, filepath.Join(".config", "lefthook.toml")},
		{ManagerLefthook, filepath.Join(".config", "lefthook.json")},
		{ManagerLefthook, filepath.Join(".config", "lefthook.jsonc")},
		{ManagerPreCommit, ".pre-commit-config.yaml"},
		{ManagerOvercommit, ".overcommit.yml"},
	}
	candidates := make([]managerConfig, 0, len(names))
	for _, name := range names {
		candidates = append(candidates, managerConfig{name.manager, filepath.Join(root, name.name)})
	}
	return candidates
}

func managerConfigPath(root string, manager Manager) string {
	for _, candidate := range managerConfigCandidates(root) {
		if candidate.manager != manager {
			continue
		}
		if _, err := os.Stat(candidate.path); err == nil {
			return candidate.path
		}
	}
	switch manager {
	case ManagerLefthook:
		return filepath.Join(root, "lefthook.yml")
	case ManagerPreCommit:
		return filepath.Join(root, ".pre-commit-config.yaml")
	case ManagerOvercommit:
		return filepath.Join(root, ".overcommit.yml")
	default:
		return root
	}
}

func installInstructions(manager Manager, configPath string) string {
	switch manager {
	case ManagerLefthook:
		return fmt.Sprintf("Lefthook manages this repository. Add this command to the commit-msg section in %s:\n\n  prosecheck:\n    run: prosecheck check --strict \"{1}\"\n\nThen run `lefthook install`.", configPath)
	case ManagerPreCommit:
		return fmt.Sprintf("pre-commit manages this repository. Add a local commit-msg hook to %s:\n\n- repo: local\n  hooks:\n    - id: prosecheck\n      name: prosecheck\n      entry: prosecheck check --strict\n      language: system\n      stages: [commit-msg]\n\nThen run `pre-commit install --hook-type commit-msg`.", configPath)
	case ManagerOvercommit:
		return fmt.Sprintf("Overcommit manages this repository. Add this entry under CommitMsg in %s:\n\n  Prosecheck:\n    enabled: true\n    required_executable: prosecheck\n    command: [prosecheck, check, --strict]\n\nThen run `overcommit --install`. Then sign the configuration.", configPath)
	default:
		return "This hook manager needs a commit-msg entry that runs `prosecheck check --strict` with the message file."
	}
}

func removeInstructions(manager Manager, configPath string) string {
	return fmt.Sprintf("%s manages this repository. Remove the prosecheck commit-msg entry from %s.", manager, configPath)
}
