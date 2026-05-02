package diffmodel

import "testing"

func TestSideFromPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   byte
		want Side
	}{
		{'+', SideHead},
		{'-', SideBase},
		{' ', SideHead},
	}
	for _, c := range cases {
		if got := SideFromPrefix(c.in); got != c.want {
			t.Errorf("SideFromPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAssignLineNumbers_Mixed(t *testing.T) {
	t.Parallel()
	// Hunk header `@@ -10,3 +20,4 @@` describing:
	//   ctx, del, add, add, ctx
	h := &Hunk{
		BaseStart: 10, BaseLines: 3,
		HeadStart: 20, HeadLines: 4,
		Lines: []Line{
			{Prefix: ' ', Text: "a"},
			{Prefix: '-', Text: "b"},
			{Prefix: '+', Text: "B1"},
			{Prefix: '+', Text: "B2"},
			{Prefix: ' ', Text: "c"},
		},
	}
	AssignLineNumbers(h)

	want := []struct {
		base, head int
	}{
		{10, 20}, // ctx
		{11, 0},  // del: base advances only
		{0, 21},  // add: head advances only
		{0, 22},  // add
		{12, 23}, // ctx
	}
	for i, w := range want {
		got := h.Lines[i]
		if got.BaseLine != w.base || got.HeadLine != w.head {
			t.Errorf("line %d = (base=%d head=%d), want (base=%d head=%d)",
				i, got.BaseLine, got.HeadLine, w.base, w.head)
		}
	}
}

func TestAssignLineNumbers_AdditionOnly(t *testing.T) {
	t.Parallel()
	// New file: `@@ -0,0 +1,2 @@` with two added lines.
	h := &Hunk{
		BaseStart: 0, BaseLines: 0,
		HeadStart: 1, HeadLines: 2,
		Lines: []Line{
			{Prefix: '+', Text: "x"},
			{Prefix: '+', Text: "y"},
		},
	}
	AssignLineNumbers(h)
	if h.Lines[0].HeadLine != 1 || h.Lines[1].HeadLine != 2 {
		t.Errorf("head numbers wrong: %+v", h.Lines)
	}
	if h.Lines[0].BaseLine != 0 || h.Lines[1].BaseLine != 0 {
		t.Errorf("base numbers should be zero: %+v", h.Lines)
	}
}

func TestAssignLineNumbers_DeletionOnly(t *testing.T) {
	t.Parallel()
	// Deleted file: `@@ -1,2 +0,0 @@` with two removed lines.
	h := &Hunk{
		BaseStart: 1, BaseLines: 2,
		HeadStart: 0, HeadLines: 0,
		Lines: []Line{
			{Prefix: '-', Text: "x"},
			{Prefix: '-', Text: "y"},
		},
	}
	AssignLineNumbers(h)
	if h.Lines[0].BaseLine != 1 || h.Lines[1].BaseLine != 2 {
		t.Errorf("base numbers wrong: %+v", h.Lines)
	}
	if h.Lines[0].HeadLine != 0 || h.Lines[1].HeadLine != 0 {
		t.Errorf("head numbers should be zero: %+v", h.Lines)
	}
}
