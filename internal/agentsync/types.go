package agentsync

type Options struct {
	Check   bool
	Repo    bool
	All     string
	Adopt   string
	Force   bool
	Watch   bool
	SkipMCP bool // 内部：仅规范/Skill 变化时由 --watch 设置，避免覆盖工具侧 MCP
}

type Target struct {
	Path string
	Mode string
	// Detect 是判断该 runtime 是否已安装的标志目录：为空表示无条件同步；
	// 非空且该目录不存在时，视为该 runtime 未安装，跳过同步，绝不为其创建文件。
	Detect string
}

type Config struct {
	Source       string
	Targets      []Target
	SkillSource  string
	SkillTargets []SkillTarget
	MCPSource    string
	MCPTargets   []MCPTarget
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
	Path string
	// Detect 语义同 Target.Detect：runtime 未安装时跳过，不创建其 skill 根目录。
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
