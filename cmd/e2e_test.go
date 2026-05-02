package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tanifumiya/sitatame/internal/review"
	"github.com/tanifumiya/sitatame/internal/tui"
)

// TestE2E_AgentConsumablePromotedReview demonstrates the PRD success condition
// 2 happy path end-to-end: a reviewer presses `s`, the orchestrator captures
// `SITATAME_REVIEW=<path>` from stdout, opens the Markdown, and finds enough
// structured information for an agent to act on (4 comment kinds + review
// body + base / head SHA in the front matter).
func TestE2E_AgentConsumablePromotedReview(t *testing.T) {
	dir, baseSHA := newRepo(t)
	chdir(t, dir)

	env, stdout, stderr := envWithRunner(os.Stdin, func(_ Env, opts TUIOptions) (TUIResult, error) {
		// Simulate the reviewer leaving a complete set of comments.
		r := opts.Review
		r.ReviewComment = "overall ok, see notes"
		r.Comments = []review.Comment{
			{
				Anchor: review.Anchor{Kind: review.KindFile, Path: "b", Side: review.SideHead, Blob: "blobB"},
				State:  review.StateOpen,
				Body:   "file-level note",
			},
			{
				Anchor: review.Anchor{Kind: review.KindLine, Path: "b", Side: review.SideHead, Line: 1, Blob: "blobB"},
				State:  review.StateOpen,
				Body:   "line note",
			},
			{
				Anchor: review.Anchor{Kind: review.KindRange, Path: "b", Side: review.SideHead, LineStart: 1, LineEnd: 1, Blob: "blobB"},
				State:  review.StateOpen,
				Body:   "range note",
			},
			{
				Anchor: review.Anchor{Kind: review.KindReview},
				State:  review.StateOpen,
				Body:   "review-kind note (carries top-level wrap-up)",
			},
		}
		r.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		return TUIResult{Review: r, Reason: tui.QuitPromote}, nil
	})

	if code := RunRoot(env, nil); code != 0 {
		t.Fatalf("RunRoot exit = %d, want 0; stderr=%q", code, stderr.String())
	}

	out := stdout.String()
	const prefix = "SITATAME_REVIEW="
	if !strings.HasPrefix(out, prefix) {
		t.Fatalf("stdout did not start with %s: %q", prefix, out)
	}
	finalPath := strings.TrimSpace(strings.TrimPrefix(out, prefix))
	if !filepath.IsAbs(finalPath) {
		t.Fatalf("captured path is not absolute: %q", finalPath)
	}

	// The orchestrator now reads the file and decodes it back into a Review.
	body, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("read promoted review: %v", err)
	}
	got, err := review.Decode(body)
	if err != nil {
		t.Fatalf("decode promoted review: %v", err)
	}

	// Front matter sanity for an agent consumer.
	if got.Schema != 1 {
		t.Errorf("schema = %d, want 1", got.Schema)
	}
	if got.Branch != "feature" {
		t.Errorf("branch = %q, want feature", got.Branch)
	}
	if got.Base.SHA != baseSHA || got.Base.Ref == "" {
		t.Errorf("base ref/sha missing: %+v", got.Base)
	}
	if got.Head.Ref != "HEAD" || got.Head.SHA == "" {
		t.Errorf("head ref/sha missing: %+v", got.Head)
	}
	if got.ReviewComment == "" {
		t.Errorf("review_comment must survive round trip")
	}

	// All four kinds must round-trip; agents key on Anchor.Kind.
	want := map[review.Kind]bool{
		review.KindFile: false, review.KindLine: false,
		review.KindRange: false, review.KindReview: false,
	}
	for _, c := range got.Comments {
		if _, ok := want[c.Kind]; ok {
			want[c.Kind] = true
		}
	}
	for k, present := range want {
		if !present {
			t.Errorf("kind %q missing from promoted review", k)
		}
	}
}
