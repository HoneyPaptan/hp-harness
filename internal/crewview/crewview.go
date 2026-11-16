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
	Title   string
	Status  string
	RunID   string
	Changed string
}

type Snapshot struct {
	ID    string
	Label string
	Done  bool
	Rows  []Row
}

type Chunk struct {
	Text    string
	Offset  int
	Waiting bool
	Ended   bool
	Trailer []string
}
