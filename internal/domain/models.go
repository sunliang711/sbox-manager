package domain

import "time"

// GlobalConfig 表示 agent 全局配置。
type GlobalConfig struct {
	Version      int              `yaml:"version" json:"version"`
	ExternalHost string           `yaml:"external_host" json:"external_host"`
	Paths        PathsConfig      `yaml:"paths" json:"paths"`
	PortRanges   PortRangesConfig `yaml:"port_ranges" json:"port_ranges"`
	Defaults     DefaultsConfig   `yaml:"defaults" json:"defaults"`
	Security     SecurityConfig   `yaml:"security" json:"security"`
}

// PathsConfig 表示 agent 使用的受管目录。
type PathsConfig struct {
	Bin       string `yaml:"bin" json:"bin"`
	Rules     string `yaml:"rules" json:"rules"`
	Instances string `yaml:"instances" json:"instances"`
	Runtime   string `yaml:"runtime" json:"runtime"`
	Generated string `yaml:"generated" json:"generated"`
	Publish   string `yaml:"publish" json:"publish"`
	Traffic   string `yaml:"traffic" json:"traffic"`
	Downloads string `yaml:"downloads" json:"downloads"`
	Logs      string `yaml:"logs" json:"logs"`
}

// PortRangesConfig 表示自动端口分配范围集合。
type PortRangesConfig struct {
	Inbound    PortRange `yaml:"inbound" json:"inbound"`
	LocalSocks PortRange `yaml:"local_socks" json:"local_socks"`
	LocalHTTP  PortRange `yaml:"local_http" json:"local_http"`
	API        PortRange `yaml:"api" json:"api"`
}

// PortRange 表示闭区间端口范围。
type PortRange struct {
	Start int `yaml:"start" json:"start"`
	End   int `yaml:"end" json:"end"`
}

// DefaultsConfig 表示 agent 的默认运行参数。
type DefaultsConfig struct {
	LogLevel string                `yaml:"log_level" json:"log_level"`
	API      APIConfig             `yaml:"api" json:"api"`
	ClashAPI APIConfig             `yaml:"clash_api" json:"clash_api"`
	Traffic  TrafficDefaultsConfig `yaml:"traffic" json:"traffic"`
}

// APIConfig 表示 sing-box API 或 Clash API 监听配置。
type APIConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Listen  string `yaml:"listen" json:"listen"`
	Token   string `yaml:"token,omitempty" json:"token,omitempty"`
}

// TrafficDefaultsConfig 表示全局 traffic 默认采集配置。
type TrafficDefaultsConfig struct {
	Enabled                bool               `yaml:"enabled" json:"enabled"`
	Timezone               string             `yaml:"timezone" json:"timezone"`
	RetentionDays          int                `yaml:"retention_days" json:"retention_days"`
	DailyMinRetentionDays  int                `yaml:"daily_min_retention_days" json:"daily_min_retention_days"`
	MonthlyRetentionMonths int                `yaml:"monthly_retention_months" json:"monthly_retention_months"`
	TimeoutSeconds         int                `yaml:"timeout_seconds" json:"timeout_seconds"`
	Timer                  TrafficTimerConfig `yaml:"timer" json:"timer"`
}

// TrafficTimerConfig 表示 traffic 周期任务的 cron 配置。
type TrafficTimerConfig struct {
	Hourly  string `yaml:"hourly" json:"hourly"`
	Daily   string `yaml:"daily" json:"daily"`
	Monthly string `yaml:"monthly" json:"monthly"`
}

// SecurityConfig 表示安全相关的全局开关。
type SecurityConfig struct {
	RequireAuthForPublicSocksHTTP bool `yaml:"require_auth_for_public_socks_http" json:"require_auth_for_public_socks_http"`
	AllowNoauthPublic             bool `yaml:"allow_noauth_public" json:"allow_noauth_public"`
}

// Instance 表示单个 sing-box 实例配置。
type Instance struct {
	Name      string                `yaml:"name" json:"name"`
	Enabled   bool                  `yaml:"enabled" json:"enabled"`
	Role      string                `yaml:"role" json:"role"`
	Labels    []string              `yaml:"labels" json:"labels"`
	API       APIConfig             `yaml:"api" json:"api"`
	Inbounds  []Inbound             `yaml:"inbounds" json:"inbounds"`
	Outbounds []Outbound            `yaml:"outbounds" json:"outbounds"`
	Groups    []Group               `yaml:"groups" json:"groups"`
	Route     RouteConfig           `yaml:"route" json:"route"`
	Traffic   InstanceTrafficConfig `yaml:"traffic" json:"traffic"`
}

// Inbound 表示面向客户端的入口协议配置。
type Inbound struct {
	Name         string             `yaml:"name" json:"name"`
	Type         string             `yaml:"type" json:"type"`
	Listen       string             `yaml:"listen" json:"listen"`
	Port         int                `yaml:"port" json:"port"`
	Tag          string             `yaml:"tag" json:"tag"`
	UDP          bool               `yaml:"udp" json:"udp"`
	Method       string             `yaml:"method,omitempty" json:"method,omitempty"`
	TLS          TLSConfig          `yaml:"tls" json:"tls"`
	Transport    TransportConfig    `yaml:"transport" json:"transport"`
	Auth         AuthConfig         `yaml:"auth" json:"auth"`
	Users        []InboundUser      `yaml:"users" json:"users"`
	Subscription SubscriptionConfig `yaml:"subscription" json:"subscription"`
}

// AuthConfig 表示 socks/http 等协议的认证配置。
type AuthConfig struct {
	Type     string `yaml:"type" json:"type"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"password"`
}

// InboundUser 表示 vmess 或 shadowsocks inbound 的用户凭据。
type InboundUser struct {
	Name     string `yaml:"name" json:"name"`
	UUID     string `yaml:"uuid" json:"uuid"`
	Password string `yaml:"password" json:"password"`
	Method   string `yaml:"method" json:"method"`
	Flow     string `yaml:"flow" json:"flow"`
	AlterID  int    `yaml:"alter_id" json:"alter_id"`
	Remark   string `yaml:"remark" json:"remark"`
	Tag      string `yaml:"tag" json:"tag"`
}

// SubscriptionConfig 表示 inbound 导出订阅节点的配置。
type SubscriptionConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	User    string `yaml:"user" json:"user"`
	Server  string `yaml:"server" json:"server"`
	Remark  string `yaml:"remark" json:"remark"`
	Region  string `yaml:"region" json:"region"`
}

// Outbound 表示出站代理或直连目标。
type Outbound struct {
	Name      string          `yaml:"name" json:"name"`
	Type      string          `yaml:"type" json:"type"`
	Ref       string          `yaml:"ref" json:"ref"`
	Server    string          `yaml:"server" json:"server"`
	Port      int             `yaml:"port" json:"port"`
	UUID      string          `yaml:"uuid" json:"uuid"`
	Password  string          `yaml:"password" json:"password"`
	Method    string          `yaml:"method" json:"method"`
	Security  string          `yaml:"security" json:"security"`
	Flow      string          `yaml:"flow" json:"flow"`
	AlterID   int             `yaml:"alter_id" json:"alter_id"`
	Auth      AuthConfig      `yaml:"auth" json:"auth"`
	TLS       TLSConfig       `yaml:"tls" json:"tls"`
	Network   string          `yaml:"network" json:"network"`
	Transport TransportConfig `yaml:"transport" json:"transport"`
}

// TLSConfig 表示连接的 TLS 客户端配置。
type TLSConfig struct {
	Enabled    bool     `yaml:"enabled" json:"enabled"`
	ServerName string   `yaml:"server_name" json:"server_name"`
	Insecure   bool     `yaml:"insecure" json:"insecure"`
	ALPN       []string `yaml:"alpn" json:"alpn"`
}

// TransportConfig 表示 VMess/VLESS 使用的 V2Ray transport 配置。
type TransportConfig struct {
	Type                string            `yaml:"type" json:"type,omitempty"`
	Host                string            `yaml:"host" json:"host,omitempty"`
	Hosts               []string          `yaml:"hosts" json:"hosts,omitempty"`
	Path                string            `yaml:"path" json:"path,omitempty"`
	Method              string            `yaml:"method" json:"method,omitempty"`
	Headers             map[string]string `yaml:"headers" json:"headers,omitempty"`
	IdleTimeout         string            `yaml:"idle_timeout" json:"idle_timeout,omitempty"`
	PingTimeout         string            `yaml:"ping_timeout" json:"ping_timeout,omitempty"`
	MaxEarlyData        int               `yaml:"max_early_data" json:"max_early_data,omitempty"`
	EarlyDataHeaderName string            `yaml:"early_data_header_name" json:"early_data_header_name,omitempty"`
	ServiceName         string            `yaml:"service_name" json:"service_name,omitempty"`
	PermitWithoutStream bool              `yaml:"permit_without_stream" json:"permit_without_stream,omitempty"`
}

// Group 表示 selector 或 urltest 出站集合。
type Group struct {
	Name      string   `yaml:"name" json:"name"`
	Type      string   `yaml:"type" json:"type"`
	Outbounds []string `yaml:"outbounds" json:"outbounds"`
	URL       string   `yaml:"url" json:"url"`
	Interval  int      `yaml:"interval" json:"interval"`
	Tolerance int      `yaml:"tolerance" json:"tolerance"`
}

// RouteConfig 表示默认出站和结构化路由规则。
type RouteConfig struct {
	Default string      `yaml:"default" json:"default"`
	Rules   []RouteRule `yaml:"rules" json:"rules"`
}

// RouteRule 表示面向用户抽象的路由规则。
type RouteRule struct {
	Type     string   `yaml:"type" json:"type"`
	Values   []string `yaml:"values" json:"values"`
	Outbound string   `yaml:"outbound" json:"outbound"`
}

// InstanceTrafficConfig 表示实例级 traffic 采集开关和范围。
type InstanceTrafficConfig struct {
	Enabled bool     `yaml:"enabled" json:"enabled"`
	Scopes  []string `yaml:"scopes" json:"scopes"`
}

// SubConfig 表示 sboxsub 服务配置。
type SubConfig struct {
	Version       int           `yaml:"version" json:"version"`
	Listen        string        `yaml:"listen" json:"listen"`
	Access        AccessConfig  `yaml:"access" json:"access"`
	TemplatesDir  string        `yaml:"templates_dir" json:"templates_dir"`
	WatchInterval time.Duration `yaml:"watch_interval" json:"watch_interval"`
	WatchDebounce time.Duration `yaml:"watch_debounce" json:"watch_debounce"`
	ManagedConfig ManagedConfig `yaml:"managed_config" json:"managed_config"`
}

// AccessConfig 表示订阅服务访问控制配置。
type AccessConfig struct {
	Type  string `yaml:"type" json:"type"`
	Token string `yaml:"token" json:"token"`
}

// ManagedConfig 表示 Surge Managed Config 输出配置。
type ManagedConfig struct {
	Enabled       bool   `yaml:"enabled" json:"enabled"`
	PublicBaseURL string `yaml:"public_base_url" json:"public_base_url"`
	Interval      int    `yaml:"interval" json:"interval"`
	Strict        bool   `yaml:"strict" json:"strict"`
}

// TrafficConfig 表示独立 traffic 配置文件可承载的采集配置。
type TrafficConfig struct {
	Version                int                `yaml:"version" json:"version"`
	Enabled                bool               `yaml:"enabled" json:"enabled"`
	Timezone               string             `yaml:"timezone" json:"timezone"`
	RetentionDays          int                `yaml:"retention_days" json:"retention_days"`
	DailyMinRetentionDays  int                `yaml:"daily_min_retention_days" json:"daily_min_retention_days"`
	MonthlyRetentionMonths int                `yaml:"monthly_retention_months" json:"monthly_retention_months"`
	TimeoutSeconds         int                `yaml:"timeout_seconds" json:"timeout_seconds"`
	Timer                  TrafficTimerConfig `yaml:"timer" json:"timer"`
}

// SubscriptionInput 表示 agent 输出给订阅服务的输入文件。
type SubscriptionInput struct {
	InputSchema  string             `yaml:"input_schema" json:"input_schema"`
	InputVersion int                `yaml:"input_version" json:"input_version"`
	Source       string             `yaml:"source" json:"source"`
	GeneratedAt  string             `yaml:"generated_at" json:"generated_at"`
	ExternalHost string             `yaml:"external_host" json:"external_host"`
	Nodes        []SubscriptionNode `yaml:"nodes" json:"nodes"`
}

// SubscriptionNode 表示一个订阅节点。
type SubscriptionNode struct {
	ID        string                 `yaml:"id" json:"id"`
	User      string                 `yaml:"user" json:"user"`
	Protocol  string                 `yaml:"protocol" json:"protocol"`
	Server    string                 `yaml:"server" json:"server"`
	Port      int                    `yaml:"port" json:"port"`
	Tag       string                 `yaml:"tag" json:"tag"`
	Remark    string                 `yaml:"remark" json:"remark"`
	Region    string                 `yaml:"region" json:"region"`
	UUID      string                 `yaml:"uuid" json:"uuid"`
	Network   string                 `yaml:"network" json:"network"`
	Security  string                 `yaml:"security" json:"security"`
	Flow      string                 `yaml:"flow" json:"flow"`
	AlterID   int                    `yaml:"alter_id" json:"alter_id"`
	Method    string                 `yaml:"method" json:"method"`
	Password  string                 `yaml:"password" json:"password"`
	Auth      AuthConfig             `yaml:"auth" json:"auth"`
	TLS       TLSConfig              `yaml:"tls" json:"tls"`
	Transport TransportConfig        `yaml:"transport" json:"transport"`
	UDP       bool                   `yaml:"udp" json:"udp"`
	Native    map[string]interface{} `yaml:"native" json:"native"`
}

// SubscriptionIndex 表示订阅服务加载后的索引骨架。
type SubscriptionIndex struct {
	IndexSchema  string             `yaml:"index_schema" json:"index_schema"`
	IndexVersion int                `yaml:"index_version" json:"index_version"`
	Sources      []string           `yaml:"sources" json:"sources"`
	Nodes        []SubscriptionNode `yaml:"nodes" json:"nodes"`
}

// BundleManifest 表示订阅 bundle 的 manifest。
type BundleManifest struct {
	BundleSchema    string            `yaml:"bundle_schema" json:"bundle_schema"`
	BundleVersion   int               `yaml:"bundle_version" json:"bundle_version"`
	Source          string            `yaml:"source" json:"source"`
	GeneratedAt     string            `yaml:"generated_at" json:"generated_at"`
	InputsSHA256    map[string]string `yaml:"inputs_sha256" json:"inputs_sha256"`
	TemplateVersion string            `yaml:"template_version" json:"template_version"`
	Access          AccessConfig      `yaml:"access" json:"access"`
}

// BackupManifest 表示 agent 配置备份 manifest。
type BackupManifest struct {
	BackupSchema  string            `yaml:"backup_schema" json:"backup_schema"`
	BackupVersion int               `yaml:"backup_version" json:"backup_version"`
	GeneratedAt   string            `yaml:"generated_at" json:"generated_at"`
	FilesSHA256   map[string]string `yaml:"files_sha256" json:"files_sha256"`
}
