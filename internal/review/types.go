package review

import (
	"time"

	"gopkg.in/yaml.v3"
)

type State string

const (
	StateOpen     State = "open"
	StateResolved State = "resolved"
	StateStale    State = "stale"
)

func (s State) Valid() bool {
	switch s {
	case StateOpen, StateResolved, StateStale:
		return true
	}
	return false
}

type Kind string

const (
	KindReview Kind = "review"
	KindFile   Kind = "file"
	KindLine   Kind = "line"
	KindRange  Kind = "range"
)

func (k Kind) Valid() bool {
	switch k {
	case KindReview, KindFile, KindLine, KindRange:
		return true
	}
	return false
}

type Side string

const (
	SideHead Side = "head"
	SideBase Side = "base"
)

func (s Side) Valid() bool {
	return s == SideHead || s == SideBase
}

type Ref struct {
	Ref string `yaml:"ref"`
	SHA string `yaml:"sha"`
}

type FileMeta struct {
	Path       string `yaml:"path"`
	BlobBase   string `yaml:"blob_base,omitempty"`
	BlobHead   string `yaml:"blob_head,omitempty"`
	Status     string `yaml:"status,omitempty"`
	RenameFrom string `yaml:"rename_from,omitempty"`
	RenameTo   string `yaml:"rename_to,omitempty"`
	Similarity int    `yaml:"similarity,omitempty"`

	// Extras holds keys not modeled by struct fields so they survive a
	// decode/encode round-trip. It is populated by codec.Decode and consumed by
	// codec.Encode; callers usually leave it nil.
	Extras map[string]*yaml.Node `yaml:"-"`
}

type Anchor struct {
	AnchorID   string `yaml:"anchor_id"`
	Kind       Kind   `yaml:"kind"`
	Path       string `yaml:"path"`
	Side       Side   `yaml:"side,omitempty"`
	Blob       string `yaml:"blob,omitempty"`
	Line       int    `yaml:"line,omitempty"`
	LineStart  int    `yaml:"line_start,omitempty"`
	LineEnd    int    `yaml:"line_end,omitempty"`
	RenameFrom string `yaml:"rename_from,omitempty"`
	RenameTo   string `yaml:"rename_to,omitempty"`
	Similarity int    `yaml:"similarity,omitempty"`
}

type Comment struct {
	Anchor `yaml:",inline"`
	State  State  `yaml:"state"`
	Body   string `yaml:"body"`

	// Extras holds keys not modeled by struct fields (including unknown anchor
	// fields, since Anchor is inlined here). See FileMeta.Extras.
	Extras map[string]*yaml.Node `yaml:"-"`
}

type Review struct {
	Schema        int        `yaml:"schema"`
	ID            string     `yaml:"id"`
	CreatedAt     time.Time  `yaml:"created_at"`
	Branch        string     `yaml:"branch"`
	Base          Ref        `yaml:"base"`
	Head          Ref        `yaml:"head"`
	Files         []FileMeta `yaml:"files,omitempty"`
	ReviewComment string     `yaml:"review_comment,omitempty"`
	Comments      []Comment  `yaml:"comments,omitempty"`

	// Extras holds top-level keys not modeled here. See FileMeta.Extras.
	Extras map[string]*yaml.Node `yaml:"-"`

	// Body is the Markdown body that follows the front matter. The codec keeps
	// it verbatim for round-trip; renderers may regenerate it from comments.
	Body string `yaml:"-"`
}
