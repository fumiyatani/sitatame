package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, repoRoot, body string) {
	t.Helper()
	dir := filepath.Join(repoRoot, DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFromRepo_FileMissing_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if cfg == nil {
		t.Fatal("cfg == nil")
	}
	if cfg.Base.Default != "" || len(cfg.Base.Candidates) != 0 {
		t.Errorf("expected zero BaseConfig; got %+v", cfg.Base)
	}
	if warn.Len() != 0 {
		t.Errorf("unexpected warnings: %s", warn.String())
	}
}

func TestLoadFromRepo_EmptyRepoRoot_ReturnsZero(t *testing.T) {
	cfg, err := LoadFromRepo("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "" || len(cfg.Base.Candidates) != 0 {
		t.Errorf("expected zero BaseConfig; got %+v", cfg.Base)
	}
}

func TestLoadFromRepo_BaseDefaultAndCandidates(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default: "origin/develop"
  candidates:
    - "origin/develop"
    - "origin/main"
    - "main"
`)
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "origin/develop" {
		t.Errorf("Default = %q, want origin/develop", cfg.Base.Default)
	}
	want := []string{"origin/develop", "origin/main", "main"}
	if len(cfg.Base.Candidates) != len(want) {
		t.Fatalf("Candidates len = %d, want %d (%v)", len(cfg.Base.Candidates), len(want), cfg.Base.Candidates)
	}
	for i, c := range want {
		if cfg.Base.Candidates[i] != c {
			t.Errorf("Candidates[%d] = %q, want %q", i, cfg.Base.Candidates[i], c)
		}
	}
	if warn.Len() != 0 {
		t.Errorf("unexpected warnings: %s", warn.String())
	}
}

func TestLoadFromRepo_UnknownTopLevelKey_Warns(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default: "main"
mystery:
  value: 1
`)
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "main" {
		t.Errorf("Default = %q, want main", cfg.Base.Default)
	}
	if !strings.Contains(warn.String(), `unknown key "mystery"`) {
		t.Errorf("expected unknown-key warning; got %q", warn.String())
	}
}

func TestLoadFromRepo_ReservedSection_WarnsAndIgnores(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default: "main"
display:
  theme: dark
keybinds:
  quit: q
`)
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "main" {
		t.Errorf("Default = %q, want main", cfg.Base.Default)
	}
	got := warn.String()
	if !strings.Contains(got, `section "display" is reserved`) {
		t.Errorf("expected reserved-section warning for display; got %q", got)
	}
	if !strings.Contains(got, `section "keybinds" is reserved`) {
		t.Errorf("expected reserved-section warning for keybinds; got %q", got)
	}
}

func TestLoadFromRepo_InvalidBaseDefaultType_WarnsAndDrops(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default:
    nested: bad
  candidates:
    - "origin/main"
`)
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "" {
		t.Errorf("Default = %q, want empty (dropped)", cfg.Base.Default)
	}
	// candidates should still survive the partial parse.
	if len(cfg.Base.Candidates) != 1 || cfg.Base.Candidates[0] != "origin/main" {
		t.Errorf("Candidates = %v, want [origin/main]", cfg.Base.Candidates)
	}
	if !strings.Contains(warn.String(), "base.default must be a string") {
		t.Errorf("expected warning about base.default type; got %q", warn.String())
	}
}

func TestLoadFromRepo_InvalidCandidatesType_WarnsAndDrops(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default: "main"
  candidates: "not-a-list"
`)
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "main" {
		t.Errorf("Default = %q, want main (preserved)", cfg.Base.Default)
	}
	if len(cfg.Base.Candidates) != 0 {
		t.Errorf("Candidates = %v, want empty (dropped)", cfg.Base.Candidates)
	}
	if !strings.Contains(warn.String(), "base.candidates must be a list") {
		t.Errorf("expected warning about base.candidates type; got %q", warn.String())
	}
}

func TestLoadFromRepo_MalformedYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default: "main
  candidates: [
`)
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err == nil {
		t.Fatalf("expected parse error; got nil (cfg=%+v)", cfg)
	}
	// Cfg returned alongside the error is the zero value so callers can
	// degrade gracefully without nil-deref.
	if cfg == nil {
		t.Fatal("cfg should be non-nil even on parse error")
	}
}

func TestLoadFromRepo_TopLevelNotMapping_WarnsAndIgnores(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "- one\n- two\n")
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "" || len(cfg.Base.Candidates) != 0 {
		t.Errorf("expected zero base; got %+v", cfg.Base)
	}
	if !strings.Contains(warn.String(), "top-level value is not a mapping") {
		t.Errorf("expected top-level mapping warning; got %q", warn.String())
	}
}

func TestLoadFromRepo_EmptyFile_ReturnsZeroNoWarn(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "")
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "" || len(cfg.Base.Candidates) != 0 {
		t.Errorf("expected zero base; got %+v", cfg.Base)
	}
	if warn.Len() != 0 {
		t.Errorf("expected no warnings on empty file; got %q", warn.String())
	}
}

func TestLoadFromRepo_UnknownBaseField_Warns(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default: "main"
  bogus: 1
`)
	var warn bytes.Buffer
	cfg, err := LoadFromRepo(dir, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "main" {
		t.Errorf("Default = %q, want main", cfg.Base.Default)
	}
	if !strings.Contains(warn.String(), "unknown key base.bogus") {
		t.Errorf("expected unknown base field warning; got %q", warn.String())
	}
}

func TestLoadFromRepo_NilWarnWriter_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `base:
  default: "main"
mystery: 1
`)
	cfg, err := LoadFromRepo(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Base.Default != "main" {
		t.Errorf("Default = %q, want main", cfg.Base.Default)
	}
}
