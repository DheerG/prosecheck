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

type Config struct {
	Subject  SubjectConfig     `json:"subject"`
	Body     BodyConfig        `json:"body"`
	Rules    map[string]string `json:"rules"`
	Semantic SemanticConfig    `json:"semantic"`
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
	Endpoint     string `json:"endpoint"`
	Model        string `json:"model"`
	Timeout      string `json:"timeout"`
	MaxDiffBytes int    `json:"maxDiffBytes"`
}

func Default() Config {
	return Config{
		Subject: SubjectConfig{MaxLength: 72, MinLength: 10},
		Body:    BodyConfig{MaxLineLength: 100, MaxSentenceWords: 25},
		Rules:   map[string]string{},
		Semantic: SemanticConfig{
			Endpoint: "http://127.0.0.1:11434/v1",
			Timeout:  "20s", MaxDiffBytes: 12000,
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
	if c.Semantic.Endpoint == "" {
		return errors.New("semantic.endpoint cannot be empty")
	}
	if _, err := time.ParseDuration(c.Semantic.Timeout); err != nil {
		return fmt.Errorf("semantic.timeout: %w", err)
	}
	if c.Semantic.MaxDiffBytes < 0 {
		return errors.New("semantic.maxDiffBytes cannot be less than zero")
	}
	for code, severity := range c.Rules {
		switch strings.ToLower(severity) {
		case "off", "error", "warning", "info":
		default:
			return fmt.Errorf("rules.%s must be off, error, warning, or info", code)
		}
	}
	return nil
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
	if value := os.Getenv("PROSECHECK_ENDPOINT"); value != "" {
		cfg.Semantic.Endpoint = value
	}
	if value := os.Getenv("PROSECHECK_MODEL"); value != "" {
		cfg.Semantic.Model = value
	}
}
