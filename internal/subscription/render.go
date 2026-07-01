package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/flosch/pongo2/v6"

	"github.com/sunliang711/sbox-manager/internal/domain"
	builtintemplates "github.com/sunliang711/sbox-manager/templates"
	"gopkg.in/yaml.v3"
)

const defaultTestURL = "http://www.gstatic.com/generate_204"

const (
	surgeSkipProxy        = "127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, localhost, *.local"
	surgeProxyListIconURL = "https://raw.githubusercontent.com/sunliang711/icons/main/surge.webp"
	surgeAutoIconURL      = "https://raw.githubusercontent.com/sunliang711/icons/main/auto.png"
)

var (
	registerFiltersOnce sync.Once
	registerFiltersErr  error
	templateTokenRe     = regexp.MustCompile(`(?s)(\{\{.*?\}\}|\{%.*?%\})`)
	templateForTagRe    = regexp.MustCompile(`^for\s+([A-Za-z_][A-Za-z0-9_]*)\s+in\s+(.+)$`)
	identifierPathRe    = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*`)
	stringLiteralRe     = regexp.MustCompile(`"[^"\\]*(?:\\.[^"\\]*)*"|'[^'\\]*(?:\\.[^'\\]*)*'`)
	regionPrefixPattern = regexp.MustCompile(`^\[([A-Z]{2})\]|^([A-Z]{2})[-_]`)
	commonRegionCodes   = []string{"HK", "JP", "US", "DE", "TW", "KR", "SG", "GB", "CA", "AU", "FR", "NL", "TR", "IN"}
	regionMetadata      = map[string]map[string]string{
		"HK": {"name": "🇭🇰 香港节点", "icon_url": "https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Rounded_Rectangle/Hong_Kong.png"},
		"JP": {"name": "🇯🇵 日本节点", "icon_url": "https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Rounded_Rectangle/Japan.png"},
		"US": {"name": "🇺🇸 美国节点", "icon_url": "https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Rounded_Rectangle/United_States.png"},
		"DE": {"name": "🇩🇪 德国节点", "icon_url": "https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Rounded_Rectangle/Germany.png"},
		"TW": {"name": "🇨🇳 台湾节点", "icon_url": "https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Rounded_Rectangle/China.png"},
		"KR": {"name": "🇰🇷 韩国节点", "icon_url": "https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Rounded_Rectangle/South_Korea.png"},
		"SG": {"name": "🇸🇬 新加坡节点", "icon_url": "https://raw.githubusercontent.com/Semporia/Hand-Painted-icon/master/Rounded_Rectangle/Singapore.png"},
		"GB": {"name": "🇬🇧 英国节点"},
		"CA": {"name": "🇨🇦 加拿大节点"},
		"AU": {"name": "🇦🇺 澳大利亚节点"},
		"FR": {"name": "🇫🇷 法国节点"},
		"NL": {"name": "🇳🇱 荷兰节点"},
		"TR": {"name": "🇹🇷 土耳其节点"},
		"IN": {"name": "🇮🇳 印度节点"},
	}
)

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

// ClashProxyGroup 表示 Clash proxy-groups 条目。
type ClashProxyGroup struct {
	Name     string   `yaml:"name" json:"name"`
	Type     string   `yaml:"type" json:"type"`
	URL      string   `yaml:"url,omitempty" json:"url,omitempty"`
	Interval int      `yaml:"interval,omitempty" json:"interval,omitempty"`
	Strategy string   `yaml:"strategy,omitempty" json:"strategy,omitempty"`
	Proxies  []string `yaml:"proxies" json:"proxies"`
}

type yamlNodePair struct {
	key   *yaml.Node
	value *yaml.Node
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
	source, err := r.loadTemplate(name)
	if err != nil {
		return nil, err
	}
	context, err := buildRenderContext(format, user, nodes, options)
	if err != nil {
		return nil, err
	}
	rendered, err := renderJinjaTemplate(name, source, context)
	if err != nil {
		return nil, err
	}
	return []byte(rendered), nil
}

// loadTemplate 按规格顺序查找自定义模板，不存在时使用内置模板。
func (r *Renderer) loadTemplate(name string) (string, error) {
	if err := validateTemplateName(name); err != nil {
		return "", err
	}
	for _, candidate := range []string{
		filepath.Join(r.TemplatesDir, "sub", name),
		filepath.Join(r.TemplatesDir, name),
		filepath.Join(r.BaseDir, "templates", "sub", name),
	} {
		data, err := osReadFile(candidate)
		if err == nil {
			return string(data), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read subscription template %s: %w", candidate, err)
		}
	}
	data, err := builtintemplates.SubFS.ReadFile(filepath.ToSlash(filepath.Join("sub", name)))
	if err != nil {
		return "", fmt.Errorf("missing built-in template %s: %w", name, err)
	}
	return string(data), nil
}

// buildRenderContext 将节点转换为模板上下文。
func buildRenderContext(format Format, user string, nodes []domain.SubscriptionNode, options RenderOptions) (map[string]interface{}, error) {
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
		return nil, err
	}
	surgeNodes := FilterNodesForFormat(FormatSurge, nodes)
	managedStrict := "false"
	if options.Config.ManagedConfig.Strict {
		managedStrict = "true"
	}
	return map[string]interface{}{
		"user":                     user,
		"generated_at":             generatedAt.Format(time.RFC3339),
		"sources":                  append([]string(nil), options.Sources...),
		"nodes":                    append([]domain.SubscriptionNode(nil), nodes...),
		"proxies":                  clashProxies(nodes),
		"proxy_names":              proxyNames,
		"proxy_groups":             clashProxyGroups(proxyNames),
		"clash_rules":              append([]string(nil), defaultClashRules()...),
		"surge_proxy_lines":        surgeProxyLines(surgeNodes),
		"surge_proxy_names":        proxyNamesForNodes(surgeNodes),
		"surge_region_groups":      surgeRegionGroups(surgeNodes),
		"surge_rules":              append([]string(nil), defaultSurgeRules()...),
		"test_url":                 defaultTestURL,
		"surge_skip_proxy":         surgeSkipProxy,
		"surge_proxylist_icon_url": surgeProxyListIconURL,
		"surge_auto_icon_url":      surgeAutoIconURL,
		"managed_config_url":       managedConfigURL(format, user, options),
		"managed_config_interval":  options.Config.ManagedConfig.Interval,
		"managed_config_strict":    managedStrict,
		"sing_box_outbounds_json":  outboundsJSON,
	}, nil
}

// clashProxies 将通用节点转换为 Clash YAML 节点。
func clashProxies(nodes []domain.SubscriptionNode) []*yaml.Node {
	proxies := make([]*yaml.Node, 0, len(nodes))
	for _, node := range nodes {
		pairs := []yamlNodePair{
			yamlPair("name", yamlString(node.Remark)),
			yamlPair("type", yamlString(clashProtocolType(node.Protocol))),
			yamlPair("server", yamlString(node.Server)),
			yamlPair("port", yamlInt(node.Port)),
		}
		switch node.Protocol {
		case "vmess":
			pairs = append(pairs,
				yamlPair("uuid", yamlString(node.UUID)),
				yamlPair("alterId", yamlInt(0)),
				yamlPair("cipher", yamlString("auto")),
				yamlPair("network", yamlString(clashNetwork(node))),
			)
		case "shadowsocks":
			pairs = append(pairs,
				yamlPair("cipher", yamlString(node.Method)),
				yamlPair("password", yamlString(node.Password)),
			)
		case "socks5":
			if node.Auth.Type == "password" {
				pairs = append(pairs,
					yamlPair("username", yamlString(node.Auth.Username)),
					yamlPair("password", yamlString(node.Auth.Password)),
				)
			}
		case "http":
			if node.Auth.Type == "password" {
				pairs = append(pairs,
					yamlPair("username", yamlString(node.Auth.Username)),
					yamlPair("password", yamlString(node.Auth.Password)),
				)
			}
		default:
			continue
		}
		if node.UDP && supportsSubscriptionUDPProtocol(node.Protocol) {
			pairs = append(pairs, yamlPair("udp", yamlBool(true)))
		}
		proxies = append(proxies, yamlMapping(pairs...))
	}
	return proxies
}

// clashProxyGroups 生成与 proxystack-go 默认模板一致的核心 Clash 分组。
func clashProxyGroups(proxyNames []string) []ClashProxyGroup {
	return []ClashProxyGroup{
		{Name: "AllProxy", Type: "select", Proxies: append(append([]string{"auto", "loadbalance"}, proxyNames...), "DIRECT")},
		{Name: "loadbalance", Type: "load-balance", URL: defaultTestURL, Interval: 300, Strategy: "round-robin", Proxies: append([]string(nil), proxyNames...)},
		{Name: "auto", Type: "url-test", URL: defaultTestURL, Interval: 300, Proxies: append([]string(nil), proxyNames...)},
		{Name: "Final", Type: "select", Proxies: append(append([]string{"AllProxy", "DIRECT"}, proxyNames...), []string{}...)},
	}
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

// surgeRegionGroups 按 region 生成 Jinja 模板使用的地区分组上下文。
func surgeRegionGroups(nodes []domain.SubscriptionNode) []map[string]interface{} {
	grouped := map[string][]string{"OTHER": {}}
	for _, region := range commonRegionCodes {
		grouped[region] = []string{}
	}
	extraRegions := map[string]bool{}
	for _, node := range nodes {
		region := resolveNodeRegion(node)
		if _, ok := grouped[region]; !ok {
			grouped[region] = []string{}
			extraRegions[region] = true
		}
		grouped[region] = append(grouped[region], node.Remark)
	}
	orderedRegions := append([]string(nil), commonRegionCodes...)
	extras := make([]string, 0, len(extraRegions))
	for region := range extraRegions {
		extras = append(extras, region)
	}
	sort.Strings(extras)
	orderedRegions = append(orderedRegions, extras...)
	orderedRegions = append(orderedRegions, "OTHER")
	groups := make([]map[string]interface{}, 0)
	for _, region := range orderedRegions {
		proxyNames := grouped[region]
		if len(proxyNames) == 0 {
			continue
		}
		groups = append(groups, map[string]interface{}{
			"region":      region,
			"name":        regionGroupName(region),
			"icon_url":    regionIconURL(region),
			"proxy_names": proxyNames,
		})
	}
	return groups
}

// proxyNamesForNodes 提取节点名称列表。
func proxyNamesForNodes(nodes []domain.SubscriptionNode) []string {
	proxyNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		proxyNames = append(proxyNames, node.Remark)
	}
	return proxyNames
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
	if tls := singBoxTLS(node.TLS, node.Protocol == "anytls"); len(tls) > 0 {
		outbound["tls"] = tls
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

// singBoxTLS 将订阅节点 TLS 配置转换为 sing-box outbound TLS 对象。
func singBoxTLS(tls domain.TLSConfig, forceEnabled bool) map[string]interface{} {
	realityEnabled := tls.Reality.Enabled
	utlsEnabled := tls.UTLS.Enabled
	if !tls.Enabled && !forceEnabled && !realityEnabled && !utlsEnabled {
		return nil
	}
	result := map[string]interface{}{
		"enabled": true,
	}
	if tls.ServerName != "" {
		result["server_name"] = tls.ServerName
	}
	if tls.Insecure {
		result["insecure"] = true
	}
	if len(tls.ALPN) > 0 {
		result["alpn"] = append([]string(nil), tls.ALPN...)
	}
	if reality := singBoxTLSReality(tls.Reality); len(reality) > 0 {
		result["reality"] = reality
	}
	if tls.UTLS.Enabled {
		utls := map[string]interface{}{
			"enabled": true,
		}
		if tls.UTLS.Fingerprint != "" {
			utls["fingerprint"] = tls.UTLS.Fingerprint
		}
		result["utls"] = utls
	}
	return result
}

// singBoxTLSReality 转换 outbound 侧 REALITY 公钥配置。
func singBoxTLSReality(reality domain.RealityConfig) map[string]interface{} {
	if !reality.Enabled {
		return nil
	}
	result := map[string]interface{}{
		"enabled": true,
	}
	if reality.PublicKey != "" {
		result["public_key"] = reality.PublicKey
	}
	if reality.ShortID != "" {
		result["short_id"] = reality.ShortID
	} else if len(reality.ShortIDs) > 0 {
		result["short_id"] = reality.ShortIDs[0]
	}
	return result
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
	} else if transport.Type == "httpupgrade" && transport.Host != "" {
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
		return base + "/surge_sub/" + url.PathEscape(options.Config.Access.Token) + "/" + url.PathEscape(user)
	}
	return base + "/surge_sub/" + url.PathEscape(user)
}

// templateName 返回格式对应的默认模板文件名。
func templateName(format Format) (string, error) {
	switch format {
	case FormatClash:
		return "clash.yaml.j2", nil
	case FormatPremiumClash:
		return "premium-clash.yaml.j2", nil
	case FormatSurge:
		return "surge.conf.j2", nil
	case FormatSingBox:
		return "sing-box.json.j2", nil
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

// renderJinjaTemplate 使用 Pongo2 渲染 Jinja 模板。
func renderJinjaTemplate(name string, text string, context map[string]interface{}) (string, error) {
	if err := validateTemplateVariables(text, context); err != nil {
		return "", err
	}
	if err := registerTemplateFilters(); err != nil {
		return "", err
	}
	tmpl, err := pongo2.FromString(text)
	if err != nil {
		return "", fmt.Errorf("parse subscription template %s: %w", name, err)
	}
	rendered, err := tmpl.Execute(pongo2.Context(context))
	if err != nil {
		return "", fmt.Errorf("render subscription template %s: %w", name, err)
	}
	return rendered, nil
}

// registerTemplateFilters 注册 Jinja 模板需要的自定义过滤器。
func registerTemplateFilters() error {
	registerFiltersOnce.Do(func() {
		if err := pongo2.RegisterFilter("yaml_block", yamlBlockFilter); err != nil && !strings.Contains(err.Error(), "already registered") {
			registerFiltersErr = err
			return
		}
		if err := pongo2.RegisterFilter("tojson", toJSONFilter); err != nil && !strings.Contains(err.Error(), "already registered") {
			registerFiltersErr = err
			return
		}
	})
	return registerFiltersErr
}

// yamlBlockFilter 将对象渲染为缩进后的 YAML 块。
func yamlBlockFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(in.Interface()); err != nil {
		return nil, &pongo2.Error{Sender: "filter:yaml_block", OrigError: err}
	}
	if err := encoder.Close(); err != nil {
		return nil, &pongo2.Error{Sender: "filter:yaml_block", OrigError: err}
	}
	text := strings.TrimRight(buffer.String(), "\n")
	indent := 2
	if param != nil && param.IsInteger() {
		indent = param.Integer()
	}
	if indent > 0 {
		prefix := strings.Repeat(" ", indent)
		lines := strings.Split(text, "\n")
		for index, line := range lines {
			if line != "" {
				lines[index] = prefix + line
			}
		}
		text = strings.Join(lines, "\n")
	}
	return pongo2.AsSafeValue(text), nil
}

// toJSONFilter 将模板变量渲染为 JSON 字符串。
func toJSONFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	text, err := jsonMarshalString(in.Interface())
	if err != nil {
		return nil, &pongo2.Error{Sender: "filter:tojson", OrigError: err}
	}
	return pongo2.AsSafeValue(text), nil
}

// validateTemplateVariables 在渲染前检查 Jinja 模板是否引用了未知变量。
func validateTemplateVariables(source string, context map[string]interface{}) error {
	rootScope := map[string]bool{}
	for key := range context {
		rootScope[key] = true
	}
	scopes := []map[string]bool{rootScope}
	for _, token := range templateTokenRe.FindAllString(source, -1) {
		switch {
		case strings.HasPrefix(token, "{{"):
			if err := validateTemplateExpression(trimTemplateToken(token, "{{", "}}"), scopes); err != nil {
				return err
			}
		case strings.HasPrefix(token, "{%"):
			tag := trimTemplateToken(token, "{%", "%}")
			switch {
			case strings.HasPrefix(tag, "for "):
				match := templateForTagRe.FindStringSubmatch(tag)
				if match == nil {
					return fmt.Errorf("subscription template contains unsupported for tag")
				}
				if err := validateTemplateExpression(match[2], scopes); err != nil {
					return err
				}
				scopes = append(scopes, map[string]bool{match[1]: true})
			case tag == "endfor":
				if len(scopes) == 1 {
					return fmt.Errorf("subscription template contains unexpected endfor")
				}
				scopes = scopes[:len(scopes)-1]
			case strings.HasPrefix(tag, "if "):
				if err := validateTemplateExpression(strings.TrimSpace(strings.TrimPrefix(tag, "if ")), scopes); err != nil {
					return err
				}
			case strings.HasPrefix(tag, "elif "):
				if err := validateTemplateExpression(strings.TrimSpace(strings.TrimPrefix(tag, "elif ")), scopes); err != nil {
					return err
				}
			}
		}
	}
	if len(scopes) != 1 {
		return fmt.Errorf("subscription template contains unclosed for tag")
	}
	return nil
}

// validateTemplateExpression 检查表达式中的根变量是否可用。
func validateTemplateExpression(expression string, scopes []map[string]bool) error {
	for _, root := range rootIdentifiers(expression) {
		if !templateVariableAvailable(root, scopes) {
			return fmt.Errorf("subscription template references undefined variable: %s", root)
		}
	}
	return nil
}

// rootIdentifiers 返回表达式中出现的根变量名。
func rootIdentifiers(expression string) []string {
	segments := strings.Split(expression, "|")
	expression = stringLiteralRe.ReplaceAllString(segments[0], " ")
	seen := map[string]bool{}
	roots := make([]string, 0)
	for _, match := range identifierPathRe.FindAllString(expression, -1) {
		root := strings.SplitN(match, ".", 2)[0]
		if root == "" || templateKeyword(root) || seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}

// templateVariableAvailable 判断变量在当前作用域链中是否存在。
func templateVariableAvailable(name string, scopes []map[string]bool) bool {
	for index := len(scopes) - 1; index >= 0; index-- {
		if scopes[index][name] {
			return true
		}
	}
	return false
}

// templateKeyword 判断标识符是否是 Jinja 表达式关键字。
func templateKeyword(value string) bool {
	switch value {
	case "and", "or", "not", "in", "is", "true", "false", "none", "nil":
		return true
	default:
		return false
	}
}

// trimTemplateToken 去掉 Jinja token 的边界符和空白裁剪标记。
func trimTemplateToken(token string, open string, close string) string {
	value := strings.TrimPrefix(strings.TrimSuffix(token, close), open)
	return strings.Trim(value, " \t\r\n-")
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

// yamlPair 创建一个稳定顺序的 YAML key/value 对。
func yamlPair(key string, value *yaml.Node) yamlNodePair {
	return yamlNodePair{key: yamlString(key), value: value}
}

// yamlMapping 创建一个保持字段顺序的 YAML mapping。
func yamlMapping(pairs ...yamlNodePair) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, pair := range pairs {
		node.Content = append(node.Content, pair.key, pair.value)
	}
	return node
}

// yamlString 创建 YAML 字符串节点。
func yamlString(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// yamlInt 创建 YAML 整数节点。
func yamlInt(value int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", value)}
}

// yamlBool 创建 YAML 布尔节点。
func yamlBool(value bool) *yaml.Node {
	if value {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}
}

// clashProtocolType 返回 Clash 订阅协议名称。
func clashProtocolType(protocol string) string {
	if protocol == "shadowsocks" {
		return "ss"
	}
	return protocol
}

// supportsSubscriptionUDPProtocol 判断默认订阅节点协议是否支持 udp 字段。
func supportsSubscriptionUDPProtocol(protocol string) bool {
	switch protocol {
	case "vmess", "shadowsocks", "socks5":
		return true
	default:
		return false
	}
}

// resolveNodeRegion 返回节点地区，缺失时从 remark 前缀推断。
func resolveNodeRegion(node domain.SubscriptionNode) string {
	if node.Region != "" {
		return node.Region
	}
	matches := regionPrefixPattern.FindStringSubmatch(node.Remark)
	if len(matches) == 3 {
		if matches[1] != "" {
			return matches[1]
		}
		return matches[2]
	}
	return "OTHER"
}

// regionGroupName 返回地区分组展示名。
func regionGroupName(region string) string {
	if region == "OTHER" {
		return "🌐 其他地区"
	}
	if metadata, ok := regionMetadata[region]; ok {
		return metadata["name"]
	}
	return region + "地区"
}

// regionIconURL 返回地区分组图标地址。
func regionIconURL(region string) string {
	if metadata, ok := regionMetadata[region]; ok {
		return metadata["icon_url"]
	}
	return ""
}

// defaultClashRules 返回普通 Clash 模板的兜底规则列表。
func defaultClashRules() []string {
	return []string{
		"DOMAIN-SUFFIX,local,DIRECT",
		"DOMAIN,localhost,DIRECT",
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"IP-CIDR,100.64.0.0/10,DIRECT,no-resolve",
		"GEOIP,CN,DIRECT",
		"MATCH,Final",
	}
}

// defaultSurgeRules 返回 Surge 模板的兜底规则列表。
func defaultSurgeRules() []string {
	return []string{
		"DOMAIN-SUFFIX,local,DIRECT",
		"DOMAIN,localhost,DIRECT",
		"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
		"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		"IP-CIDR,100.64.0.0/10,DIRECT,no-resolve",
		"GEOIP,CN,DIRECT",
		"FINAL,FinalList",
	}
}

// jsonMarshalString 返回不转义 HTML 的 JSON 字符串。
func jsonMarshalString(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// osReadFile 包装 os.ReadFile，便于测试替换和保持模板查找逻辑清晰。
var osReadFile = os.ReadFile
