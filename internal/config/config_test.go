package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesFileWithDefaults(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	if err := os.WriteFile(path, []byte(`{
  "subject": {"maxLength": 60},
  "semantic": {"enabled": true}
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
	if cfg.Semantic.Model != "bonsai-8b" || cfg.Semantic.Timeout != "20s" {
		t.Fatalf("semantic defaults were not merged: %#v", cfg.Semantic)
	}
	if cfg.Semantic.Runtime != "managed" {
		t.Fatalf("expected the managed runtime, got %q", cfg.Semantic.Runtime)
	}
	if !cfg.SimpleEnglish.Enabled || cfg.SimpleEnglish.Mode != SimpleEnglishPragmatic || cfg.SimpleEnglish.Severity != "warning" {
		t.Fatalf("simple English defaults were not merged: %#v", cfg.SimpleEnglish)
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
	if cfg.Semantic.Runtime != "external" {
		t.Fatalf("an endpoint override must select the external runtime: %#v", cfg.Semantic)
	}
}

func TestLoadTreatsAnExistingEndpointAsExternal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{
  "semantic": {
    "enabled": true,
    "endpoint": "http://127.0.0.1:9000/v1",
    "model": "older-model"
  }
}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Semantic.Runtime != "external" {
		t.Fatalf("expected an external runtime, got %q", cfg.Semantic.Runtime)
	}
}

func TestValidateRejectsBadRuleSeverity(t *testing.T) {
	cfg := Default()
	cfg.Rules["PC001"] = "sometimes"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid severity to fail")
	}
}

func TestValidateRejectsBadSimpleEnglishSeverity(t *testing.T) {
	cfg := Default()
	cfg.SimpleEnglish.Severity = "sometimes"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an invalid Simple English severity to fail")
	}
}

func TestValidateRejectsBadSimpleEnglishMode(t *testing.T) {
	cfg := Default()
	cfg.SimpleEnglish.Mode = "sometimes"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an invalid Simple English mode to fail")
	}
}

func TestWriteCreatesACompleteConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	cfg := Default()
	cfg.Semantic.Enabled = true
	cfg.SimpleEnglish.Allow = []string{"OAuth"}
	if err := Write(path, cfg); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var written Config
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	if !written.Semantic.Enabled || !written.SimpleEnglish.Enabled || len(written.SimpleEnglish.Allow) != 1 {
		t.Fatalf("unexpected configuration: %#v", written)
	}
}
