package diffmodel

import "testing"

func TestStatusValid(t *testing.T) {
	t.Parallel()
	for _, s := range []Status{
		StatusAdded, StatusModified, StatusDeleted,
		StatusRenamed, StatusCopied, StatusTypeChange,
	} {
		if !s.Valid() {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range []Status{0, 'X', 'a'} {
		if s.Valid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestStatusString(t *testing.T) {
	t.Parallel()
	if got := StatusModified.String(); got != "M" {
		t.Errorf("StatusModified.String() = %q, want M", got)
	}
	if got := Status(0).String(); got != "?" {
		t.Errorf("zero Status.String() = %q, want ?", got)
	}
}

func TestFileZeroValue(t *testing.T) {
	t.Parallel()
	var f File
	if f.Status != 0 || f.Binary || f.Similarity != 0 || f.Hunks != nil {
		t.Errorf("File zero value not zero: %+v", f)
	}
	if f.DisplayPath() != "" {
		t.Errorf("zero DisplayPath = %q, want empty", f.DisplayPath())
	}
}

func TestFileDisplayPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   File
		want string
	}{
		{"added uses post", File{Status: StatusAdded, PostPath: "new.go"}, "new.go"},
		{"deleted uses pre", File{Status: StatusDeleted, PrePath: "old.go"}, "old.go"},
		{"renamed uses post", File{Status: StatusRenamed, PrePath: "old.go", PostPath: "new.go"}, "new.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.DisplayPath(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHunkAndLineZero(t *testing.T) {
	t.Parallel()
	var h Hunk
	if h.BaseStart != 0 || h.HeadStart != 0 || h.Lines != nil || h.Header != "" {
		t.Errorf("Hunk zero value not zero: %+v", h)
	}
	var l Line
	if l.Prefix != 0 || l.Text != "" || l.BaseLine != 0 || l.HeadLine != 0 {
		t.Errorf("Line zero value not zero: %+v", l)
	}
}
