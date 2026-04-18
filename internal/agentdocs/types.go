package agentdocs

// Kind identifies the type of canonical source document.
type Kind string

const (
	// KindContract identifies the repository contract source document.
	KindContract Kind = "contract"
	// KindSkill identifies a skill source document.
	KindSkill Kind = "skill"
)

// Source is one canonical docs/agents document.
type Source struct {
	Path    string
	Kind    Kind
	Slug    string
	Title   string
	Summary string
	Body    string
}

// Output is one generated provider projection.
type Output struct {
	Path    string
	Content string
}

// Mismatch describes one out-of-date generated file.
type Mismatch struct {
	Path     string
	Expected string
	Observed string
	Missing  bool
	Stale    bool
}
