package diffmodel

type Status byte

const (
	StatusAdded      Status = 'A'
	StatusModified   Status = 'M'
	StatusDeleted    Status = 'D'
	StatusRenamed    Status = 'R'
	StatusCopied     Status = 'C'
	StatusTypeChange Status = 'T'
)

func (s Status) Valid() bool {
	switch s {
	case StatusAdded, StatusModified, StatusDeleted,
		StatusRenamed, StatusCopied, StatusTypeChange:
		return true
	}
	return false
}

func (s Status) String() string {
	if !s.Valid() {
		return "?"
	}
	return string(s)
}

type Side byte

const (
	SideHead Side = '+'
	SideBase Side = '-'
)

type Line struct {
	Prefix   byte
	Text     string
	BaseLine int
	HeadLine int
}

type Hunk struct {
	BaseStart int
	BaseLines int
	HeadStart int
	HeadLines int
	Header    string
	Lines     []Line
}

type File struct {
	Status     Status
	PrePath    string
	PostPath   string
	BlobBase   string
	BlobHead   string
	ModeBase   string
	ModeHead   string
	RenameFrom string
	RenameTo   string
	Similarity int
	Binary     bool
	Hunks      []Hunk
}

func (f File) DisplayPath() string {
	if f.PostPath != "" {
		return f.PostPath
	}
	return f.PrePath
}
