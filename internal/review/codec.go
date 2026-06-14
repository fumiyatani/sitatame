package review

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const frontMatterDelim = "---"

// knownTopKeys lists the top-level YAML keys handled by Review struct fields.
// Anything not in this set lands in Review.Extras.
var knownTopKeys = map[string]struct{}{
	"schema":         {},
	"id":             {},
	"created_at":     {},
	"branch":         {},
	"base":           {},
	"head":           {},
	"files":          {},
	"review_comment": {},
	"comments":       {},
}

var knownFileKeys = map[string]struct{}{
	"path":        {},
	"blob_base":   {},
	"blob_head":   {},
	"status":      {},
	"rename_from": {},
	"rename_to":   {},
	"similarity":  {},
}

// Comment merges Anchor (inline) + Comment fields, so the known set spans both.
var knownCommentKeys = map[string]struct{}{
	"anchor_id":   {},
	"kind":        {},
	"path":        {},
	"side":        {},
	"blob":        {},
	"line":        {},
	"line_start":  {},
	"line_end":    {},
	"rename_from": {},
	"rename_to":   {},
	"similarity":  {},
	"state":       {},
	"body":        {},
}

// Decode parses a review file (YAML front matter + Markdown body) into a
// Review. Unknown YAML keys at top / file / comment level are preserved in
// the corresponding Extras fields so they survive Encode.
func Decode(b []byte) (Review, error) {
	fm, body, err := splitFrontMatter(b)
	if err != nil {
		return Review{}, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(fm, &root); err != nil {
		return Review{}, fmt.Errorf("front matter yaml: %w", err)
	}
	// yaml.Unmarshal on a document returns a DocumentNode wrapping a
	// MappingNode at Content[0].
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return Review{}, fmt.Errorf("front matter: expected mapping, got empty document")
	}
	mapNode := root.Content[0]
	if mapNode.Kind != yaml.MappingNode {
		return Review{}, fmt.Errorf("front matter: top-level must be a mapping")
	}

	var r Review
	if err := mapNode.Decode(&r); err != nil {
		return Review{}, fmt.Errorf("front matter decode: %w", err)
	}
	r.Body = body
	r.Extras = collectExtras(mapNode, knownTopKeys)

	// Per-file and per-comment extras: walk the matching nodes.
	if files := childNode(mapNode, "files"); files != nil && files.Kind == yaml.SequenceNode {
		for i, item := range files.Content {
			if i >= len(r.Files) || item.Kind != yaml.MappingNode {
				continue
			}
			r.Files[i].Extras = collectExtras(item, knownFileKeys)
		}
	}
	if cs := childNode(mapNode, "comments"); cs != nil && cs.Kind == yaml.SequenceNode {
		for i, item := range cs.Content {
			if i >= len(r.Comments) || item.Kind != yaml.MappingNode {
				continue
			}
			r.Comments[i].Extras = collectExtras(item, knownCommentKeys)
		}
	}

	return r, nil
}

// Encode serializes a Review back into the front-matter + Markdown form.
// Unknown keys held in Extras are merged back into the corresponding mappings.
func Encode(r Review) ([]byte, error) {
	// Encode the struct directly into a yaml.Node without going through an
	// intermediate bytes representation. The old approach of Marshal→Unmarshal
	// (bytes round-trip) was fragile: certain body strings caused go-yaml to
	// emit a scalar form that the same library could not re-parse (issue #76).
	// Node.Encode(v) populates the receiver as a MappingNode in one step,
	// avoiding the round-trip path entirely.
	root := &yaml.Node{}
	if err := root.Encode(&r); err != nil {
		return nil, fmt.Errorf("yaml encode to node: %w", err)
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("yaml encode: top-level node is not a mapping (kind=%v)", root.Kind)
	}

	mergeExtras(root, r.Extras)

	if files := childNode(root, "files"); files != nil && files.Kind == yaml.SequenceNode {
		for i, item := range files.Content {
			if i >= len(r.Files) || item.Kind != yaml.MappingNode {
				continue
			}
			mergeExtras(item, r.Files[i].Extras)
		}
	}
	if cs := childNode(root, "comments"); cs != nil && cs.Kind == yaml.SequenceNode {
		for i, item := range cs.Content {
			if i >= len(r.Comments) || item.Kind != yaml.MappingNode {
				continue
			}
			mergeExtras(item, r.Comments[i].Extras)
		}
	}

	var buf bytes.Buffer
	buf.WriteString(frontMatterDelim)
	buf.WriteByte('\n')
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("yaml encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("yaml encode close: %w", err)
	}
	buf.WriteString(frontMatterDelim)
	buf.WriteByte('\n')
	if r.Body != "" {
		// Ensure exactly one blank line between the closing delim and body.
		buf.WriteByte('\n')
		buf.WriteString(strings.TrimLeft(r.Body, "\n"))
		if !strings.HasSuffix(r.Body, "\n") {
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes(), nil
}

// splitFrontMatter splits `---\n...\n---\n<body>` into the YAML chunk and the
// Markdown body. Both leading delim and trailing delim are required.
func splitFrontMatter(b []byte) ([]byte, string, error) {
	s := string(b)
	// Tolerate a leading BOM / whitespace before the opening delim.
	trimmed := strings.TrimLeft(s, "\ufeff \t\r\n")
	if !strings.HasPrefix(trimmed, frontMatterDelim) {
		return nil, "", fmt.Errorf("missing opening %q delimiter", frontMatterDelim)
	}
	// Skip the opening line (everything up to the first newline after the delim).
	rest := trimmed[len(frontMatterDelim):]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	} else {
		return nil, "", fmt.Errorf("missing newline after opening delimiter")
	}
	// Find the closing delim line. It must appear at the start of a line.
	closeIdx := indexOfLine(rest, frontMatterDelim)
	if closeIdx < 0 {
		return nil, "", fmt.Errorf("missing closing %q delimiter", frontMatterDelim)
	}
	fm := rest[:closeIdx]
	after := rest[closeIdx+len(frontMatterDelim):]
	// Drop the newline that terminates the closing delim line, if any.
	after = strings.TrimPrefix(after, "\n")
	// Trim a single leading blank line so the body starts with content.
	body := strings.TrimLeft(after, "\n")
	return []byte(fm), body, nil
}

// indexOfLine returns the byte offset where `marker` appears at the start of a
// line within s, or -1 if not found. The match must be the entire line content
// (the marker can be followed by \n or end-of-string).
func indexOfLine(s, marker string) int {
	for i := 0; i < len(s); {
		end := strings.IndexByte(s[i:], '\n')
		var line string
		if end < 0 {
			line = s[i:]
		} else {
			line = s[i : i+end]
		}
		if line == marker {
			return i
		}
		if end < 0 {
			return -1
		}
		i += end + 1
	}
	return -1
}

func childNode(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// collectExtras returns a map of YAML nodes for keys in `m` that are not in
// `known`. Returns nil when there are no unknown keys.
func collectExtras(m *yaml.Node, known map[string]struct{}) map[string]*yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	var out map[string]*yaml.Node
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i].Value
		if _, ok := known[k]; ok {
			continue
		}
		if out == nil {
			out = map[string]*yaml.Node{}
		}
		// Deep copy via re-marshal isn't necessary since the node lifetime
		// outlives Decode's caller; the caller may mutate, but for round-trip
		// they don't.
		out[k] = m.Content[i+1]
	}
	return out
}

// mergeExtras appends extras into the mapping node, skipping keys already set.
func mergeExtras(m *yaml.Node, extras map[string]*yaml.Node) {
	if m == nil || m.Kind != yaml.MappingNode || len(extras) == 0 {
		return
	}
	existing := map[string]struct{}{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		existing[m.Content[i].Value] = struct{}{}
	}
	// Iterate in sorted order for determinism. Stdlib's map range is random.
	keys := sortedKeys(extras)
	for _, k := range keys {
		if _, dup := existing[k]; dup {
			continue
		}
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
		m.Content = append(m.Content, keyNode, extras[k])
	}
}

func sortedKeys(m map[string]*yaml.Node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// tiny n; insertion sort keeps the dep-graph minimal.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}
