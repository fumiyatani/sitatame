// Package config loads per-repository sitatame settings from
// <repo>/.sitatame/config.yaml.
//
// Phase 1 (issue #24) scope is intentionally narrow: only the [base] section
// is honored. Unknown top-level keys are accepted with a one-line stderr
// warning so future sections ([display], [keybinds], …) can be reserved
// without breaking older binaries that encounter the keys.
//
// Field-level invalid types (e.g. `base.candidates` set to a scalar instead
// of a list) are also non-fatal: the offending field is dropped, a warning is
// printed, and the remaining valid fields are still applied. This matches the
// project-wide stance that config issues should never block the TUI from
// launching — the auto-detect fallback in gitx.BaseCandidates is always
// available.
package config

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the on-disk basename of the per-repo config file, resolved
// relative to <repo>/.sitatame/.
const FileName = "config.yaml"

// DirName is the basename of the per-repo config directory. This matches the
// directory the review store treats as legacy on-disk output; the config
// loader and the legacy-dir warning coordinate via FileName so that the
// presence of *only* config.yaml does not trigger the legacy warning.
const DirName = ".sitatame"

// Config is the in-memory representation of <repo>/.sitatame/config.yaml.
// Zero value (returned when the file is absent) means "no overrides; use
// built-in defaults".
type Config struct {
	Base BaseConfig
}

// BaseConfig models the [base] section.
//
// Default is the single ref that should be tried first when the user does not
// pass an explicit base argument. It is effectively shorthand for putting one
// entry at the front of Candidates.
//
// Candidates, when non-empty, fully replaces the built-in gitx.BaseCandidates
// chain. When empty, the built-in chain is appended after Default so users
// only setting Default still benefit from the auto-detect fallback.
type BaseConfig struct {
	Default    string
	Candidates []string
}

// LoadFromRepo reads <repoRoot>/.sitatame/config.yaml and returns the parsed
// Config. A missing file is not an error: the zero Config is returned and the
// caller falls back to built-in defaults.
//
// Warnings (unknown top-level keys, invalid field types, malformed YAML
// recovered field-by-field) are written to warnTo so the CLI can route them
// to stderr while tests can capture them. Passing nil disables warnings.
//
// A hard YAML parse error (file present but not parseable at all) returns a
// non-nil error so the caller can decide whether to abort or degrade to the
// zero Config. Today cmd/root.go chooses the latter: a broken config file
// must never block the user from reviewing diffs.
func LoadFromRepo(repoRoot string, warnTo io.Writer) (*Config, error) {
	if repoRoot == "" {
		return &Config{}, nil
	}
	path := filepath.Join(repoRoot, DirName, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Config{}, nil
		}
		return &Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	return parse(data, path, warnTo)
}

// parse is the file-content-only entry point. Split out so tests can exercise
// YAML edge cases without touching the filesystem.
//
// The decode strategy is two-pass: first into a yaml.Node so we can walk the
// top-level mapping and emit "unknown key" warnings for everything outside
// the known section list, then into a typed shape for the [base] section.
// Field-level type errors inside [base] are reported individually rather than
// failing the whole parse — see decodeBase for the per-field recovery.
func parse(data []byte, sourcePath string, warnTo io.Writer) (*Config, error) {
	if len(data) == 0 {
		return &Config{}, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return &Config{}, fmt.Errorf("parse %s: %w", sourcePath, err)
	}
	cfg := &Config{}
	if root.Kind == 0 {
		// Pure-whitespace / comment-only file. Treat as empty.
		return cfg, nil
	}
	doc := &root
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return cfg, nil
		}
		doc = root.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		warn(warnTo, "%s: top-level value is not a mapping; ignoring file", sourcePath)
		return cfg, nil
	}

	knownSections := map[string]bool{
		"base": true,
		// Reserved for future use. Listing them here suppresses the
		// "unknown key" warning so users can stage config for later
		// releases without seeing spurious warnings.
		"display":  true,
		"keybinds": true,
	}

	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]
		key := keyNode.Value
		if !knownSections[key] {
			warn(warnTo, "%s: unknown key %q (ignored)", sourcePath, key)
			continue
		}
		switch key {
		case "base":
			cfg.Base = decodeBase(valNode, sourcePath, warnTo)
		case "display", "keybinds":
			// Reserved sections — parsed and discarded.
			warn(warnTo, "%s: section %q is reserved and not yet implemented (ignored)", sourcePath, key)
		}
	}
	return cfg, nil
}

// decodeBase pulls the two supported fields out of the [base] mapping with
// per-field type checks. A non-mapping value, a missing field, or a field of
// the wrong type does not abort: the offending field is dropped, a warning is
// printed, and the rest of [base] is still applied.
func decodeBase(node *yaml.Node, sourcePath string, warnTo io.Writer) BaseConfig {
	var b BaseConfig
	if node == nil || node.Kind != yaml.MappingNode {
		if node != nil {
			warn(warnTo, "%s: base section must be a mapping (ignored)", sourcePath)
		}
		return b
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		switch keyNode.Value {
		case "default":
			if valNode.Kind != yaml.ScalarNode {
				warn(warnTo, "%s: base.default must be a string (ignored)", sourcePath)
				continue
			}
			b.Default = valNode.Value
		case "candidates":
			if valNode.Kind != yaml.SequenceNode {
				warn(warnTo, "%s: base.candidates must be a list (ignored)", sourcePath)
				continue
			}
			cands := make([]string, 0, len(valNode.Content))
			ok := true
			for _, item := range valNode.Content {
				if item.Kind != yaml.ScalarNode {
					warn(warnTo, "%s: base.candidates entries must be strings (entire list ignored)", sourcePath)
					ok = false
					break
				}
				if item.Value != "" {
					cands = append(cands, item.Value)
				}
			}
			if ok {
				b.Candidates = cands
			}
		default:
			warn(warnTo, "%s: unknown key base.%s (ignored)", sourcePath, keyNode.Value)
		}
	}
	return b
}

// warn writes a single sitatame-prefixed line, but only when w is non-nil.
// The prefix matches the rest of the CLI so users see a consistent shape.
func warn(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, "sitatame: config: "+format+"\n", args...)
}
