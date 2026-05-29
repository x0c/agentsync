package agentsync

type Options struct {
	Check bool
	Repo  bool
	All   string
	Adopt string
	Force bool
}

type Target struct {
	Path string
	Mode string
}

type Config struct {
	Source       string
	Targets      []Target
	SkillSource  string
	SkillTargets []SkillTarget
}

type SkillTarget struct {
	Path string
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
	MergeDraft   string
	Backups      []string
	Repositories int
}
