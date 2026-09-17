package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const FileName = ".prosecheck.json"

const DefaultSemanticTimeout = 60 * time.Second

type Config struct {
	Subject       SubjectConfig       `json:"subject"`
	Body          BodyConfig          `json:"body"`
	Rules         map[string]string   `json:"rules"`
	SimpleEnglish SimpleEnglishConfig `json:"simpleEnglish"`
	Semantic      SemanticConfig      `json:"semantic"`
}

type SubjectConfig struct {
	MaxLength int `json:"maxLength"`
	MinLength int `json:"minLength"`
}

type BodyConfig struct {
	MaxLineLength    int `json:"maxLineLength"`
	MaxSentenceWords int `json:"maxSentenceWords"`
}

type SemanticConfig struct {
	Enabled      bool   `json:"enabled"`
	Runtime      string `json:"runtime"`
	Endpoint     string `json:"endpoint,omitempty"`
	Model        string `json:"model"`
	Timeout      string `json:"timeout"`
	MaxDiffBytes int    `json:"maxDiffBytes"`
}

type SimpleEnglishConfig struct {
	Enabled  bool     `json:"enabled"`
	Mode     string   `json:"mode"`
	Severity string   `json:"severity"`
	Allow    []string `json:"allow"`
}

const (
	SimpleEnglishPragmatic = "pragmatic"
	SimpleEnglishStrict    = "strict"
)

func Write(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".prosecheck-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func Default() Config {
	return Config{
		Subject: SubjectConfig{MaxLength: 72, MinLength: 10},
		Body:    BodyConfig{MaxLineLength: 100, MaxSentenceWords: 25},
		Rules:   map[string]string{},
		SimpleEnglish: SimpleEnglishConfig{
			Enabled: true, Mode: SimpleEnglishStrict, Severity: "warning", Allow: []string{},
		},
		Semantic: SemanticConfig{
			Runtime: "managed", Model: "ministral-3-8b",
			Timeout: DefaultSemanticTimeout.String(), MaxDiffBytes: 12000,
		},
	}
}

func Load(explicitPath string) (Config, string, error) {
	cfg := Default()
	path := explicitPath
	if path == "" {
		path = discover()
	}
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, "", err
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, "", fmt.Errorf("%s: %w", path, err)
		}
		applyLegacySemanticDefaults(data, &cfg)
	}
	applyEnvironment(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, "", err
	}
	return cfg, path, nil
}

func (c Config) Validate() error {
	if c.Subject.MaxLength < 1 {
		return errors.New("subject.maxLength must be more than zero")
	}
	if c.Subject.MinLength < 1 || c.Subject.MinLength > c.Subject.MaxLength {
		return errors.New("subject.minLength must be more than zero and less than subject.maxLength")
	}
	if c.Body.MaxLineLength < 1 || c.Body.MaxSentenceWords < 1 {
		return errors.New("body limits must be more than zero")
	}
	switch c.Semantic.Runtime {
	case "managed":
		if c.Semantic.Model != "ministral-3-8b" {
			return errors.New("semantic.model must be ministral-3-8b when semantic.runtime is managed")
		}
	case "external":
		if c.Semantic.Endpoint == "" {
			return errors.New("semantic.endpoint cannot be empty when semantic.runtime is external")
		}
	default:
		return errors.New("semantic.runtime must be managed or external")
	}
	timeout, err := time.ParseDuration(c.Semantic.Timeout)
	if err != nil {
		return fmt.Errorf("semantic.timeout: %w", err)
	}
	if timeout <= 0 {
		return errors.New("semantic.timeout must be more than zero")
	}
	if c.Semantic.MaxDiffBytes < 0 {
		return errors.New("semantic.maxDiffBytes cannot be less than zero")
	}
	if !validSeverity(c.SimpleEnglish.Severity, false) {
		return errors.New("simpleEnglish.severity must be error, warning, or info")
	}
	if c.SimpleEnglish.Mode != SimpleEnglishPragmatic && c.SimpleEnglish.Mode != SimpleEnglishStrict {
		return errors.New("simpleEnglish.mode must be pragmatic or strict")
	}
	for code, severity := range c.Rules {
		if !validSeverity(severity, true) {
			return fmt.Errorf("rules.%s must be off, error, warning, or info", code)
		}
	}
	return nil
}

func validSeverity(value string, allowOff bool) bool {
	switch strings.ToLower(value) {
	case "error", "warning", "info":
		return true
	case "off":
		return allowOff
	default:
		return false
	}
}

func discover() string {
	if root := gitRoot(); root != "" {
		candidate := filepath.Join(root, FileName)
		if fileExists(candidate) {
			return candidate
		}
	}
	current, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(current, FileName)
		if fileExists(candidate) {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func gitRoot() string {
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	command.Stderr = nil
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func applyEnvironment(cfg *Config) {
	if value := os.Getenv("PROSECHECK_TIMEOUT"); value != "" {
		cfg.Semantic.Timeout = value
	}
	if value := os.Getenv("PROSECHECK_ENDPOINT"); value != "" {
		cfg.Semantic.Endpoint = value
		cfg.Semantic.Runtime = "external"
	}
	if value := os.Getenv("PROSECHECK_MODEL"); value != "" {
		cfg.Semantic.Model = value
	}
	if value := os.Getenv("PROSECHECK_RUNTIME"); value != "" {
		cfg.Semantic.Runtime = value
	}
}

func applyLegacySemanticDefaults(data []byte, cfg *Config) {
	var raw struct {
		Semantic map[string]json.RawMessage `json:"semantic"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Semantic == nil {
		return
	}
	_, hasRuntime := raw.Semantic["runtime"]
	_, hasEndpoint := raw.Semantic["endpoint"]
	if !hasRuntime && hasEndpoint {
		cfg.Semantic.Runtime = "external"
	}
}
