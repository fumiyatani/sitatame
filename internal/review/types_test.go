package review

import "testing"

func TestStateValid(t *testing.T) {
	t.Parallel()
	for _, s := range []State{StateOpen, StateResolved, StateStale} {
		if !s.Valid() {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range []State{"", "OPEN", "closed"} {
		if State(s).Valid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestKindValid(t *testing.T) {
	t.Parallel()
	for _, k := range []Kind{KindReview, KindFile, KindLine, KindRange} {
		if !k.Valid() {
			t.Errorf("expected %q to be valid", k)
		}
	}
	if Kind("comment").Valid() {
		t.Error("Kind(\"comment\") should be invalid")
	}
}

func TestSideValid(t *testing.T) {
	t.Parallel()
	if !SideHead.Valid() || !SideBase.Valid() {
		t.Error("head/base should be valid")
	}
	if Side("").Valid() {
		t.Error("empty side should be invalid")
	}
}

func TestZeroValues(t *testing.T) {
	t.Parallel()

	var r Review
	if r.Schema != 0 || r.ID != "" || r.Branch != "" {
		t.Errorf("Review zero value not zero: %+v", r)
	}
	if r.Files != nil || r.Comments != nil {
		t.Error("zero Review should have nil slices")
	}

	var c Comment
	if c.AnchorID != "" || c.State != "" || c.Body != "" || c.Line != 0 {
		t.Errorf("Comment zero value not zero: %+v", c)
	}

	var a Anchor
	if a.AnchorID != "" || a.Kind != "" || a.Line != 0 ||
		a.LineStart != 0 || a.LineEnd != 0 || a.Similarity != 0 {
		t.Errorf("Anchor zero value not zero: %+v", a)
	}
}

func TestCommentEmbedsAnchor(t *testing.T) {
	t.Parallel()
	c := Comment{
		Anchor: Anchor{
			AnchorID: "id-1",
			Kind:     KindLine,
			Path:     "src/a.go",
			Side:     SideHead,
			Blob:     "deadbeef",
			Line:     12,
		},
		State: StateOpen,
		Body:  "nit",
	}
	if c.AnchorID != "id-1" || c.Kind != KindLine || c.Line != 12 {
		t.Errorf("anchor not promoted: %+v", c)
	}
}
