package subscription

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

const defaultTestURL = "http://www.gstatic.com/generate_204"

// Format 表示订阅输出格式。
type Format string

const (
	// FormatClash 表示 Clash 订阅输出。
	FormatClash Format = "clash"
	// FormatPremiumClash 表示 Premium Clash 订阅输出。
	FormatPremiumClash Format = "premium-clash"
	// FormatSurge 表示 Surge 订阅输出。
	FormatSurge Format = "surge"
	// FormatSingBox 表示 sing-box 订阅输出。
	FormatSingBox Format = "sing-box"
)

// Renderer 使用 Go 标准模板渲染订阅内容。
type Renderer struct {
	BaseDir      string
	TemplatesDir string
}

// RenderOptions 描述一次订阅渲染所需的外部上下文。
type RenderOptions struct {
	Config     domain.SubConfig
	RequestURL string
	Sources    []string
	Now        time.Time
}

// RenderContext 是传给 Go 模板的导出字段上下文。
type RenderContext struct {
	User                  string
	GeneratedAt           string
	Sources               []string
	Nodes                 []domain.SubscriptionNode
	ProxyNames            []string
	ClashProxies          []ClashProxy
	ClashProxyGroups      []ClashProxyGroup
	ClashRules            []string
	SurgeProxyLines       []string
	SurgeProxyNames       []string
	SurgeRegionGroups     []SurgeRegionGroup
	SurgeRules            []string
	ManagedConfigURL      string
	ManagedConfigInterval int
	ManagedConfigStrict   bool
	TestURL               string
	SingBoxOutboundsJSON  string
}

// ClashProxy 表示 Clash 代理条目的模板字段。
type ClashProxy struct {
	Name     string
	Type     string
	Server   string
	Port     int
	UUID     string
	Cipher   string
	Password string
	Network  string
	Username string
	UDP      bool
}

// ClashProxyGroup 表示 Clash proxy-groups 条目。
type ClashProxyGroup struct {
	Name    string
	Type    string
	Proxies []string
}

// SurgeRegionGroup 表示按 region 聚合的 Surge 代理组。
type SurgeRegionGroup struct {
	Region  string
	Proxies []string
}

// ParseFormat 校验并转换订阅格式名称。
func ParseFormat(value string) (Format, error) {
	switch Format(value) {
	case FormatClash, FormatPremiumClash, FormatSurge, FormatSingBox:
		return Format(value), nil
	default:
		return "", fmt.Errorf("unsupported subscription format %q", value)
	}
}

// FilterNodesForFormat 返回目标订阅格式可渲染的节点集合。
func FilterNodesForFormat(format Format, nodes []domain.SubscriptionNode) []domain.SubscriptionNode {
	if format == FormatSingBox {
		copied := make([]domain.SubscriptionNode, len(nodes))
		copy(copied, nodes)
		return copied
	}
	filtered := make([]domain.SubscriptionNode, 0, len(nodes))
	for _, node := range nodes {
		if format == FormatSurge {
			if surgeSupportsNode(node) {
				filtered = append(filtered, node)
			}
			continue
		}
		switch node.Protocol {
		case "vmess", "shadowsocks", "socks5", "http":
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// NewRenderer 创建订阅模板渲染器。
func NewRenderer(baseDir string, templatesDir string) *Renderer {
	return &Renderer{
		BaseDir:      baseDir,
		TemplatesDir: templatesDir,
	}
}

// Render 渲染指定格式和用户节点的订阅内容。
func (r *Renderer) Render(format Format, user string, nodes []domain.SubscriptionNode, options RenderOptions) ([]byte, error) {
	name, err := templateName(format)
	if err != nil {
		return nil, err
	}
	tmpl, err := r.loadTemplate(name)
	if err != nil {
		return nil, err
	}
	context, err := buildRenderContext(format, user, nodes, options)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, context); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// loadTemplate 按规格顺序查找自定义模板，不存在时使用内置模板。
func (r *Renderer) loadTemplate(name string) (*template.Template, error) {
	if err := validateTemplateName(name); err != nil {
		return nil, err
	}
	for _, candidate := range []string{
		filepath.Join(r.TemplatesDir, "sub", name),
		filepath.Join(r.TemplatesDir, name),
		filepath.Join(r.BaseDir, "templates", "sub", name),
	} {
		data, err := osReadFile(candidate)
		if err == nil {
			return parseTemplate(name, string(data))
		}
	}
	builtin, ok := builtinTemplates[name]
	if !ok {
		return nil, fmt.Errorf("missing built-in template %s", name)
	}
	return parseTemplate(name, builtin)
}

// buildRenderContext 将节点转换为模板上下文。
func buildRenderContext(format Format, user string, nodes []domain.SubscriptionNode, options RenderOptions) (RenderContext, error) {
	nodes = FilterNodesForFormat(format, nodes)
	generatedAt := options.Now
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	proxyNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		proxyNames = append(proxyNames, node.Remark)
	}
	outboundsJSON, err := singBoxOutboundsJSON(nodes)
	if err != nil {
		return RenderContext{}, err
	}
	return RenderContext{
		User:                  user,
		GeneratedAt:           generatedAt.Format(time.RFC3339),
		Sources:               append([]string(nil), options.Sources...),
		Nodes:                 append([]domain.SubscriptionNode(nil), nodes...),
		ProxyNames:            proxyNames,
		ClashProxies:          clashProxies(nodes),
		ClashProxyGroups:      []ClashProxyGroup{{Name: "proxy", Type: "select", Proxies: proxyNames}},
		ClashRules:            []string{"MATCH,proxy"},
		SurgeProxyLines:       surgeProxyLines(nodes),
		SurgeProxyNames:       proxyNames,
		SurgeRegionGroups:     surgeRegionGroups(nodes),
		SurgeRules:            []string{"FINAL,proxy"},
		ManagedConfigURL:      managedConfigURL(format, user, options),
		ManagedConfigInterval: options.Config.ManagedConfig.Interval,
		ManagedConfigStrict:   options.Config.ManagedConfig.Strict,
		TestURL:               defaultTestURL,
		SingBoxOutboundsJSON:  outboundsJSON,
	}, nil
}

// clashProxies 将通用节点转换为 Clash 模板字段。
func clashProxies(nodes []domain.SubscriptionNode) []ClashProxy {
	proxies := make([]ClashProxy, 0, len(nodes))
	for _, node := range nodes {
		proxy := ClashProxy{
			Name:     node.Remark,
			Server:   node.Server,
			Port:     node.Port,
			UUID:     node.UUID,
			Cipher:   node.Method,
			Password: node.Password,
			Network:  clashNetwork(node),
			Username: node.Auth.Username,
			UDP:      node.UDP,
		}
		switch node.Protocol {
		case "vmess":
			proxy.Type = "vmess"
			proxy.Cipher = "auto"
		case "shadowsocks":
			proxy.Type = "ss"
		case "socks5":
			proxy.Type = "socks5"
			proxy.Password = node.Auth.Password
		case "http":
			proxy.Type = "http"
			proxy.Password = node.Auth.Password
		default:
			continue
		}
		proxies = append(proxies, proxy)
	}
	return proxies
}

// surgeProxyLines 将通用节点转换为 Surge proxy 行。
func surgeProxyLines(nodes []domain.SubscriptionNode) []string {
	lines := make([]string, 0, len(nodes))
	for _, node := range nodes {
		switch node.Protocol {
		case "vmess":
			lines = append(lines, fmt.Sprintf("%s = vmess, %s, %d, username=%s%s", node.Remark, node.Server, node.Port, node.UUID, surgeVMessSuffix(node)))
		case "anytls":
			lines = append(lines, fmt.Sprintf("%s = anytls, %s, %d, password=%s", node.Remark, node.Server, node.Port, node.Password))
		case "shadowsocks":
			lines = append(lines, fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=%s", node.Remark, node.Server, node.Port, node.Method, node.Password))
		case "socks5":
			lines = append(lines, fmt.Sprintf("%s = socks5, %s, %d%s", node.Remark, node.Server, node.Port, surgeAuthSuffix(node.Auth)))
		case "http":
			lines = append(lines, fmt.Sprintf("%s = http, %s, %d%s", node.Remark, node.Server, node.Port, surgeAuthSuffix(node.Auth)))
		}
	}
	return lines
}

// surgeRegionGroups 按 region 生成 Surge 代理组上下文。
func surgeRegionGroups(nodes []domain.SubscriptionNode) []SurgeRegionGroup {
	grouped := make(map[string][]string)
	for _, node := range nodes {
		if node.Region == "" {
			continue
		}
		grouped[node.Region] = append(grouped[node.Region], node.Remark)
	}
	regions := make([]string, 0, len(grouped))
	for region := range grouped {
		regions = append(regions, region)
	}
	sort.Strings(regions)
	groups := make([]SurgeRegionGroup, 0, len(regions))
	for _, region := range regions {
		groups = append(groups, SurgeRegionGroup{Region: region, Proxies: grouped[region]})
	}
	return groups
}

// singBoxOutboundsJSON 生成 sing-box outbound JSON 片段。
func singBoxOutboundsJSON(nodes []domain.SubscriptionNode) (string, error) {
	outbounds := make([]map[string]interface{}, 0, len(nodes)+1)
	tags := make([]string, 0, len(nodes))
	for _, node := range nodes {
		outbound := singBoxOutbound(node)
		if len(outbound) == 0 {
			continue
		}
		outbounds = append(outbounds, outbound)
		if tag, ok := outbound["tag"].(string); ok && tag != "" {
			tags = append(tags, tag)
		}
	}
	if len(tags) > 0 {
		outbounds = append(outbounds, map[string]interface{}{
			"type":      "selector",
			"tag":       "proxy",
			"outbounds": tags,
		})
	}
	data, err := MarshalStable(outbounds)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// singBoxOutbound 将单个节点转换为 sing-box outbound 对象。
func singBoxOutbound(node domain.SubscriptionNode) map[string]interface{} {
	if node.Protocol == "sing-box" {
		outbound := make(map[string]interface{}, len(node.Native)+1)
		for key, value := range node.Native {
			outbound[key] = value
		}
		if _, exists := outbound["tag"]; !exists {
			outbound["tag"] = node.Tag
		}
		return outbound
	}

	outbound := map[string]interface{}{
		"type":        singBoxOutboundType(node.Protocol),
		"tag":         node.Tag,
		"server":      node.Server,
		"server_port": node.Port,
	}
	if node.UDP {
		outbound["udp"] = true
	}
	if node.TLS.Enabled || node.Protocol == "anytls" {
		outbound["tls"] = map[string]interface{}{"enabled": true}
	}
	if subscriptionNodeSupportsTransport(node.Protocol) {
		if transport := singBoxTransport(node.Transport); len(transport) > 0 {
			outbound["transport"] = transport
		}
	}
	switch node.Protocol {
	case "vmess":
		outbound["uuid"] = node.UUID
		if node.Security != "" {
			outbound["security"] = node.Security
		}
		if node.AlterID != 0 {
			outbound["alter_id"] = node.AlterID
		}
		if node.Network != "" {
			outbound["network"] = node.Network
		}
	case "vless":
		outbound["uuid"] = node.UUID
		if node.Flow != "" {
			outbound["flow"] = node.Flow
		}
	case "anytls":
		outbound["password"] = node.Password
	case "shadowsocks":
		outbound["method"] = node.Method
		outbound["password"] = node.Password
	case "socks5", "http":
		if node.Auth.Type == "password" {
			outbound["username"] = node.Auth.Username
			outbound["password"] = node.Auth.Password
		}
	}
	return outbound
}

// subscriptionNodeSupportsTransport 判断订阅节点协议是否支持 V2Ray transport。
func subscriptionNodeSupportsTransport(protocol string) bool {
	return protocol == "vmess" || protocol == "vless"
}

// singBoxTransport 将订阅节点 transport 转换为 sing-box outbound transport 对象。
func singBoxTransport(transport domain.TransportConfig) map[string]interface{} {
	if transport.Type == "" {
		return nil
	}
	result := map[string]interface{}{
		"type": transport.Type,
	}
	if transport.Type == "http" && len(transport.Hosts) > 0 {
		result["host"] = append([]string(nil), transport.Hosts...)
	} else if transport.Host != "" {
		result["host"] = transport.Host
	}
	if transport.Path != "" {
		result["path"] = transport.Path
	}
	if transport.Method != "" && transport.Type != "httpupgrade" {
		result["method"] = transport.Method
	}
	if len(transport.Headers) > 0 {
		result["headers"] = transport.Headers
	}
	if transport.IdleTimeout != "" {
		result["idle_timeout"] = transport.IdleTimeout
	}
	if transport.PingTimeout != "" {
		result["ping_timeout"] = transport.PingTimeout
	}
	if transport.MaxEarlyData != 0 {
		result["max_early_data"] = transport.MaxEarlyData
	}
	if transport.EarlyDataHeaderName != "" {
		result["early_data_header_name"] = transport.EarlyDataHeaderName
	}
	if transport.ServiceName != "" {
		result["service_name"] = transport.ServiceName
	}
	if transport.PermitWithoutStream {
		result["permit_without_stream"] = true
	}
	return result
}

// surgeSupportsNode 判断 Surge 是否支持该订阅节点协议。
func surgeSupportsNode(node domain.SubscriptionNode) bool {
	switch node.Protocol {
	case "vmess", "anytls", "shadowsocks", "socks5", "http":
		return true
	default:
		return false
	}
}

// managedConfigURL 根据配置和请求上下文生成 Surge managed config URL。
func managedConfigURL(format Format, user string, options RenderOptions) string {
	if format != FormatSurge || !options.Config.ManagedConfig.Enabled {
		return ""
	}
	if options.Config.ManagedConfig.PublicBaseURL == "" {
		return options.RequestURL
	}
	base := strings.TrimRight(options.Config.ManagedConfig.PublicBaseURL, "/")
	if options.Config.Access.Type == "token" {
		return base + "/surge/" + url.PathEscape(options.Config.Access.Token) + "/" + url.PathEscape(user)
	}
	return base + "/surge/" + url.PathEscape(user)
}

// templateName 返回格式对应的默认模板文件名。
func templateName(format Format) (string, error) {
	switch format {
	case FormatClash:
		return "clash.yaml.tmpl", nil
	case FormatPremiumClash:
		return "premium-clash.yaml.tmpl", nil
	case FormatSurge:
		return "surge.conf.tmpl", nil
	case FormatSingBox:
		return "sing-box.json.tmpl", nil
	default:
		return "", fmt.Errorf("unsupported subscription format %q", format)
	}
}

// validateTemplateName 校验模板文件名只能是安全 basename。
func validateTemplateName(name string) error {
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("template filename is unsafe: %s", name)
	}
	return nil
}

// parseTemplate 解析模板并启用缺失 key 报错。
func parseTemplate(name string, text string) (*template.Template, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Funcs(template.FuncMap{
		"quote": strconv.Quote,
	}).Parse(text)
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

// singBoxOutboundType 返回 sing-box outbound 类型名称。
func singBoxOutboundType(protocol string) string {
	if protocol == "socks5" {
		return "socks"
	}
	return protocol
}

// surgeAuthSuffix 生成 Surge 代理认证字段后缀。
func surgeAuthSuffix(auth domain.AuthConfig) string {
	if auth.Type != "password" {
		return ""
	}
	return fmt.Sprintf(", username=%s, password=%s", auth.Username, auth.Password)
}

// surgeVMessSuffix 生成 Surge VMess 的 websocket、AEAD 和加密参数。
func surgeVMessSuffix(node domain.SubscriptionNode) string {
	var parts []string
	if node.Transport.Type == "ws" {
		parts = append(parts, "ws=true")
		if node.Transport.Path != "" {
			parts = append(parts, "ws-path="+node.Transport.Path)
		}
		if len(node.Transport.Headers) > 0 {
			parts = append(parts, "ws-headers="+formatSurgeHeaders(node.Transport.Headers))
		}
	}
	if node.Security != "" {
		parts = append(parts, "encrypt-method="+node.Security)
	}
	if node.AlterID == 0 {
		parts = append(parts, "vmess-aead=true")
	}
	if len(parts) == 0 {
		return ""
	}
	return ", " + strings.Join(parts, ", ")
}

// formatSurgeHeaders 按稳定顺序生成 Surge ws-headers 参数。
func formatSurgeHeaders(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+":"+headers[key])
	}
	return strings.Join(values, "|")
}

// clashNetwork 为 Clash 文本订阅派生 VMess network 字段，不污染 sing-box network。
func clashNetwork(node domain.SubscriptionNode) string {
	if node.Transport.Type != "" {
		return node.Transport.Type
	}
	return valueOrDefault(node.Network, "tcp")
}

// valueOrDefault 返回非空值或默认值。
func valueOrDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// osReadFile 包装 os.ReadFile，便于测试替换和保持模板查找逻辑清晰。
var osReadFile = os.ReadFile

// builtinTemplates 保存 T05 内置订阅模板。
var builtinTemplates = map[string]string{
	"clash.yaml.tmpl": `proxies:
{{ range .ClashProxies }}
  - name: {{ quote .Name }}
    type: {{ .Type }}
    server: {{ quote .Server }}
    port: {{ .Port }}
{{ if .UUID }}
    uuid: {{ .UUID }}
    alterId: 0
    cipher: {{ .Cipher }}
    network: {{ .Network }}
{{ end }}
{{ if and .Cipher .Password }}
    cipher: {{ .Cipher }}
    password: {{ quote .Password }}
{{ end }}
{{ if .Username }}
    username: {{ quote .Username }}
    password: {{ quote .Password }}
{{ end }}
{{ if .UDP }}
    udp: true
{{ end }}
{{ end }}
proxy-groups:
{{ range .ClashProxyGroups }}
  - name: {{ quote .Name }}
    type: {{ .Type }}
    proxies:
{{ range .Proxies }}
      - {{ quote . }}
{{ end }}
{{ end }}
rules:
{{ range .ClashRules }}
  - {{ . }}
{{ end }}
`,
	"premium-clash.yaml.tmpl": `proxies:
{{ range .ClashProxies }}
  - name: {{ quote .Name }}
    type: {{ .Type }}
    server: {{ quote .Server }}
    port: {{ .Port }}
{{ if .UUID }}
    uuid: {{ .UUID }}
    alterId: 0
    cipher: {{ .Cipher }}
    network: {{ .Network }}
{{ end }}
{{ if and .Cipher .Password }}
    cipher: {{ .Cipher }}
    password: {{ quote .Password }}
{{ end }}
{{ if .Username }}
    username: {{ quote .Username }}
    password: {{ quote .Password }}
{{ end }}
{{ if .UDP }}
    udp: true
{{ end }}
{{ end }}
proxy-groups:
{{ range .ClashProxyGroups }}
  - name: {{ quote .Name }}
    type: {{ .Type }}
    proxies:
{{ range .Proxies }}
      - {{ quote . }}
{{ end }}
{{ end }}
rules:
{{ range .ClashRules }}
  - {{ . }}
{{ end }}
`,
	"surge.conf.tmpl": `{{ if .ManagedConfigURL }}#!MANAGED-CONFIG {{ .ManagedConfigURL }} interval={{ .ManagedConfigInterval }} strict={{ .ManagedConfigStrict }}
{{ end }}[Proxy]
{{ range .SurgeProxyLines }}
{{ . }}
{{ end }}

[Proxy Group]
proxy = select{{ range .SurgeProxyNames }}, {{ . }}{{ end }}
{{ range .SurgeRegionGroups }}
{{ .Region }} = select{{ range .Proxies }}, {{ . }}{{ end }}
{{ end }}

[Rule]
{{ range .SurgeRules }}
{{ . }}
{{ end }}
`,
	"sing-box.json.tmpl": `{
  "outbounds": {{ .SingBoxOutboundsJSON }},
  "route": {
    "final": "proxy"
  }
}
`,
}
