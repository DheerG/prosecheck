package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesFileWithDefaults(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	if err := os.WriteFile(path, []byte(`{
  "subject": {"maxLength": 60},
  "semantic": {"enabled": true, "model": "local-model"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, loadedPath, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loadedPath != path {
		t.Fatalf("expected %q, got %q", path, loadedPath)
	}
	if cfg.Subject.MaxLength != 60 || cfg.Subject.MinLength != 10 {
		t.Fatalf("defaults were not merged: %#v", cfg.Subject)
	}
	if cfg.Semantic.Model != "local-model" || cfg.Semantic.Timeout != "20s" {
		t.Fatalf("semantic defaults were not merged: %#v", cfg.Semantic)
	}
}

func TestLoadAppliesEnvironment(t *testing.T) {
	t.Setenv("PROSECHECK_ENDPOINT", "http://127.0.0.1:9999/v1")
	t.Setenv("PROSECHECK_MODEL", "environment-model")

	cfg, _, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected an explicit missing file to fail")
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Semantic.Endpoint != "http://127.0.0.1:9999/v1" || cfg.Semantic.Model != "environment-model" {
		t.Fatalf("environment was not applied: %#v", cfg.Semantic)
	}
}

func TestValidateRejectsBadRuleSeverity(t *testing.T) {
	cfg := Default()
	cfg.Rules["PC001"] = "sometimes"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid severity to fail")
	}
}
