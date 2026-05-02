package review

import "path/filepath"

const (
	rootDir    = ".sitatame"
	reviewsDir = "reviews"
	draftsDir  = "drafts"
)

// Paths resolves on-disk locations under <repo>/.sitatame for a branch.
type Paths struct {
	RepoRoot string
	Branch   string
	Slug     string
}

func NewPaths(repoRoot, branch string) Paths {
	return Paths{
		RepoRoot: repoRoot,
		Branch:   branch,
		Slug:     BranchSlug(branch),
	}
}

func (p Paths) Root() string         { return filepath.Join(p.RepoRoot, rootDir) }
func (p Paths) ReviewsRoot() string  { return filepath.Join(p.Root(), reviewsDir) }
func (p Paths) DraftsRoot() string   { return filepath.Join(p.Root(), draftsDir) }
func (p Paths) ReviewsDir() string   { return filepath.Join(p.ReviewsRoot(), p.Slug) }
func (p Paths) DraftsDir() string    { return filepath.Join(p.DraftsRoot(), p.Slug) }
func (p Paths) ReviewFile(id string) string {
	return filepath.Join(p.ReviewsDir(), id+".md")
}
func (p Paths) DraftFile(id string) string {
	return filepath.Join(p.DraftsDir(), id+".md")
}
