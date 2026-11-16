package crewview

type Start struct {
	Plan    string
	Label   string
	N       int
	Model   string
	BaseURL string
	KeyEnv  string
}

type Row struct {
	Slug    string
	Title   string
	Status  string
	RunID   string
	Changed string
	Keys    string
}

type Snapshot struct {
	ID     string
	Label  string
	Done   bool
	Paused bool
	Rows   []Row
}

type Chunk struct {
	Text    string
	Offset  int
	Waiting bool
	Ended   bool
	Trailer []string
}
