//go:build tui_e2e

package tui

import (
	"testing"

	"github.com/fumiyatani/sitatame/internal/diffmodel"
	"github.com/fumiyatani/sitatame/internal/review"
)

// TestScenario_FilePickerJumpFlow is the end-to-end teatest scenario:
// f to open, j to move down, Enter to jump. Final cursor must land on
// the second file's header row.
//
// Moved out of filepicker_test.go (which is part of the default unit-test
// suite) so the scenario harness can stay behind the `tui_e2e` build tag.
func TestScenario_FilePickerJumpFlow(t *testing.T) {
	t.Parallel()
	files := []diffmodel.File{
		pickerFile("a.go", diffmodel.StatusModified, 1, 0),
		pickerFile("b.go", diffmodel.StatusAdded, 2, 1),
	}
	// Pre-compute the expected cursor index so the assertion is explicit.
	probe := New(files, review.Review{})
	want := fileHeaderRowIndex(probe.rows, 1)
	runScenario(t, Scenario{
		Name:  "file_picker_jump_flow",
		Files: files,
		Steps: []Step{
			{
				SendKey:                "f",
				RequirePostEventOutput: true,
				Expect:                 Expectation{ViewContains: []string{"Files (2)"}},
			},
			{SendKey: "j"},
			{
				SendKey: "enter",
				Expect: Expectation{
					Cursor: intPtr(want),
					Top:    intPtr(want),
				},
			},
		},
	})
}
