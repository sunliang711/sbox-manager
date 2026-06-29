package singbox

// Config 表示本项目生成的 sing-box 配置顶层结构。
type Config struct {
	Log          Log           `json:"log"`
	DNS          DNS           `json:"dns"`
	Inbounds     []Inbound     `json:"inbounds"`
	Outbounds    []Outbound    `json:"outbounds"`
	Route        Route         `json:"route"`
	Experimental *Experimental `json:"experimental,omitempty"`
}

// Log 表示 sing-box 日志配置。
type Log struct {
	Level string `json:"level"`
}

// DNS 表示 sing-box DNS 配置。
type DNS struct {
	Servers []DNSServer `json:"servers"`
}

// DNSServer 表示 sing-box DNS server 条目。
type DNSServer struct {
	Type   string `json:"type"`
	Tag    string `json:"tag,omitempty"`
	Server string `json:"server,omitempty"`
}

// Inbound 表示 sing-box inbound 通用配置。
type Inbound struct {
	Type       string        `json:"type"`
	Tag        string        `json:"tag"`
	Listen     string        `json:"listen"`
	ListenPort int           `json:"listen_port"`
	UDP        *bool         `json:"udp,omitempty"`
	TLS        *TLS          `json:"tls,omitempty"`
	Transport  *Transport    `json:"transport,omitempty"`
	Method     string        `json:"method,omitempty"`
	Password   string        `json:"password,omitempty"`
	Users      []InboundUser `json:"users,omitempty"`
}

// InboundUser 表示 sing-box inbound 用户条目。
type InboundUser struct {
	Name     string `json:"name,omitempty"`
	UUID     string `json:"uuid,omitempty"`
	Password string `json:"password,omitempty"`
	Method   string `json:"method,omitempty"`
	Username string `json:"username,omitempty"`
	Flow     string `json:"flow,omitempty"`
	AlterID  int    `json:"alterId,omitempty"`
}

// Outbound 表示 sing-box outbound 或 outbound group 配置。
type Outbound struct {
	Type       string     `json:"type"`
	Tag        string     `json:"tag"`
	Server     string     `json:"server,omitempty"`
	ServerPort int        `json:"server_port,omitempty"`
	UUID       string     `json:"uuid,omitempty"`
	Password   string     `json:"password,omitempty"`
	Method     string     `json:"method,omitempty"`
	Security   string     `json:"security,omitempty"`
	Flow       string     `json:"flow,omitempty"`
	AlterID    int        `json:"alter_id,omitempty"`
	Username   string     `json:"username,omitempty"`
	Network    string     `json:"network,omitempty"`
	TLS        *TLS       `json:"tls,omitempty"`
	Outbounds  []string   `json:"outbounds,omitempty"`
	URL        string     `json:"url,omitempty"`
	Interval   string     `json:"interval,omitempty"`
	Tolerance  int        `json:"tolerance,omitempty"`
	Transport  *Transport `json:"transport,omitempty"`
}

// TLS 表示 sing-box outbound 的 TLS 配置。
type TLS struct {
	Enabled bool `json:"enabled"`
}

// Transport 表示 sing-box vmess 等协议的传输配置。
type Transport struct {
	Type                string            `json:"type"`
	Host                interface{}       `json:"host,omitempty"`
	Path                string            `json:"path,omitempty"`
	Method              string            `json:"method,omitempty"`
	Headers             map[string]string `json:"headers,omitempty"`
	IdleTimeout         string            `json:"idle_timeout,omitempty"`
	PingTimeout         string            `json:"ping_timeout,omitempty"`
	MaxEarlyData        int               `json:"max_early_data,omitempty"`
	EarlyDataHeaderName string            `json:"early_data_header_name,omitempty"`
	ServiceName         string            `json:"service_name,omitempty"`
	PermitWithoutStream bool              `json:"permit_without_stream,omitempty"`
}

// Route 表示 sing-box 路由配置。
type Route struct {
	Rules []RouteRule `json:"rules,omitempty"`
	Final string      `json:"final"`
}

// RouteRule 表示 sing-box 单条路由规则。
type RouteRule struct {
	Domain        []string `json:"domain,omitempty"`
	DomainSuffix  []string `json:"domain_suffix,omitempty"`
	DomainKeyword []string `json:"domain_keyword,omitempty"`
	IPCIDR        []string `json:"ip_cidr,omitempty"`
	GeoIP         []string `json:"geoip,omitempty"`
	Geosite       []string `json:"geosite,omitempty"`
	Outbound      string   `json:"outbound"`
}

// Experimental 表示 sing-box experimental 配置。
type Experimental struct {
	V2RayAPI *V2RayAPI `json:"v2ray_api,omitempty"`
}

// V2RayAPI 表示 sing-box V2Ray API 配置。
type V2RayAPI struct {
	Listen string     `json:"listen"`
	Stats  V2RayStats `json:"stats"`
}

// V2RayStats 表示 sing-box API stats 开关。
type V2RayStats struct {
	Enabled   bool     `json:"enabled"`
	Inbounds  []string `json:"inbounds,omitempty"`
	Outbounds []string `json:"outbounds,omitempty"`
	Users     []string `json:"users,omitempty"`
}
