package runtime

import "context"

const (
	// ManifestSchema 是 runtime manifest 的固定 schema 名。
	ManifestSchema = "sbox.runtime-manifest"
	// ManifestVersion 是 runtime manifest 当前版本。
	ManifestVersion = 1
)

// Action 表示 runtime plan 中单个文件的动作。
type Action string

const (
	// ActionCreate 表示需要创建受管文件。
	ActionCreate Action = "create"
	// ActionUpdate 表示需要更新受管文件或 manifest 输入 hash。
	ActionUpdate Action = "update"
	// ActionDelete 表示需要删除 manifest 中受管文件。
	ActionDelete Action = "delete"
	// ActionNoChange 表示文件和 manifest 均无需改写。
	ActionNoChange Action = "no-change"
)

// Manifest 表示 runtime manifest 文件。
type Manifest struct {
	ManifestSchema  string            `json:"manifest_schema"`
	ManifestVersion int               `json:"manifest_version"`
	ConfigSHA256    string            `json:"config_sha256"`
	InstanceSHA256  map[string]string `json:"instance_sha256"`
	GeneratedAt     string            `json:"generated_at"`
	Files           []ManifestFile    `json:"files"`
}

// ManifestFile 表示 runtime manifest 中的单个受管生成物。
type ManifestFile struct {
	Instance     string `json:"instance"`
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	SHA256       string `json:"sha256"`
	Service      string `json:"service"`
}

// DesiredFile 表示 plan 中期望存在的生成物和对应内容。
type DesiredFile struct {
	ManifestFile
	Data []byte
}

// Change 表示 plan 中单个文件的 diff 动作。
type Change struct {
	Action       Action `json:"action"`
	Instance     string `json:"instance"`
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	SHA256       string `json:"sha256,omitempty"`
	Service      string `json:"service"`
	Data         []byte `json:"-"`
}

// Plan 表示一次 runtime 预览或应用计划。
type Plan struct {
	ManifestPath   string
	RuntimeDir     string
	GeneratedDir   string
	Target         string
	ConfigSHA256   string
	InstanceSHA256 map[string]string
	Existing       *Manifest
	DesiredFiles   []DesiredFile
	Changes        []Change
	targetScope    map[string]struct{}
	fullScope      bool
}

// ApplyResult 表示 runtime apply 的执行结果。
type ApplyResult struct {
	Manifest *Manifest
	Changed  bool
}

// ConfigChecker 表示可注入的 sing-box 配置检查器。
type ConfigChecker interface {
	Check(ctx context.Context, instance string, data []byte) error
}

// ServiceManager 表示可注入的实例服务管理器。
type ServiceManager interface {
	Start(ctx context.Context, services []string) error
	Restart(ctx context.Context, services []string) error
}

// Clock 返回当前时间，便于测试固定 generated_at。
type Clock func() string
