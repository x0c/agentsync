package agentsync

type Options struct {
	Check    bool
	Repo     bool
	All      string
	Adopt    string
	Rollback string // "latest", YYYYMMDD-HHMMSS, or path under backups/; empty = unused
	Force    bool
	Watch    bool
	SkipMCP  bool // Internal: watch sets this for instruction/skill-only changes to preserve runtime MCP.
}

type Target struct {
	Path string
	Mode string
	// Detect is the runtime installation directory; an empty value disables gating.
	// If a nonempty Detect path is absent, skip synchronization without creating files.
	Detect string
}

type Config struct {
	Source       string
	Targets      []Target
	SkillSource  string
	SkillTargets []SkillTarget
	MCPSource    string
	MCPTargets   []MCPTarget
	// PolicyPath is ~/.config/agentsync/sync-policy.json (optional; missing = allow-all).
	PolicyPath string
}

type MCPTarget struct {
	Name    string
	Path    string
	Detect  string
	Dialect string
	Format  string
	Mode    string
}

type SkillTarget struct {
	// Name is the stable runtime key used in sync-policy.json (e.g. "codex").
	Name string
	Path string
	// Detect follows Target.Detect: skip absent runtimes without creating skill roots.
	Detect string
}

type TargetResult struct {
	Path   string
	Status string
	Detail string
}

type RunReport struct {
	Source       string
	Results      []TargetResult
	SkillResults []TargetResult
	MCPResults   []TargetResult
	MergeDraft   string
	Backups      []string
	Repositories int
}
