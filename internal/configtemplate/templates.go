package configtemplate

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

// templateFiles 保存 sboxctl example 和内置 add 模板共享的 YAML 片段。
//
//go:embed templates
var templateFiles embed.FS

// Context 描述模板渲染所需的少量可变值。
type Context struct {
	InstanceName           string
	Role                   string
	ShowExampleFields      bool
	IncludeProxyAuth       bool
	SubscriptionServer     string
	VmessInboundName       string
	VmessInboundPort       int
	VmessUUID              string
	VmessRemark            string
	VmessUserTag           string
	VlessUUID              string
	AnyTLSPassword         string
	ShadowsocksInboundName string
	ShadowsocksInboundPort int
	ShadowsocksPassword    string
	ShadowsocksRemark      string
	LocalSocksName         string
	LocalSocksPort         int
	LocalHTTPName          string
	LocalHTTPPort          int
	VmessOutboundName      string
	VmessOutboundUUID      string
	GroupName              string
	GroupOutbounds         string
	RefMembers             []RefMember
	RefOutboundName        string
	Ref                    string
}

// RefMember 描述 urltest 模板中的一个 ref outbound 成员。
type RefMember struct {
	OutboundName    string
	RefOutboundName string
	Ref             string
}

type snippetDefinition struct {
	ID          string
	Kind        string
	Type        string
	Description string
	Path        string
	Aliases     []string
}

var snippetDefinitions = []snippetDefinition{
	{ID: "inbound.vmess-raw", Kind: "inbound", Type: "vmess-raw", Description: "VMess raw inbound", Path: "templates/snippets/inbound/vmess-raw.yaml.tmpl", Aliases: []string{"vmess"}},
	{ID: "inbound.vmess-websocket", Kind: "inbound", Type: "vmess-websocket", Description: "VMess WebSocket inbound", Path: "templates/snippets/inbound/vmess-websocket.yaml.tmpl"},
	{ID: "inbound.vmess-grpc", Kind: "inbound", Type: "vmess-grpc", Description: "VMess gRPC inbound", Path: "templates/snippets/inbound/vmess-grpc.yaml.tmpl"},
	{ID: "inbound.vless-websocket", Kind: "inbound", Type: "vless-websocket", Description: "VLESS WebSocket inbound", Path: "templates/snippets/inbound/vless-websocket.yaml.tmpl", Aliases: []string{"vless"}},
	{ID: "inbound.vless-grpc", Kind: "inbound", Type: "vless-grpc", Description: "VLESS gRPC inbound", Path: "templates/snippets/inbound/vless-grpc.yaml.tmpl"},
	{ID: "inbound.anytls", Kind: "inbound", Type: "anytls", Description: "AnyTLS inbound", Path: "templates/snippets/inbound/anytls.yaml.tmpl"},
	{ID: "inbound.shadowsocks", Kind: "inbound", Type: "shadowsocks", Description: "Shadowsocks inbound", Path: "templates/snippets/inbound/shadowsocks.yaml.tmpl", Aliases: []string{"ss", "shadowsocks22", "shadowsocks2022"}},
	{ID: "inbound.socks5", Kind: "inbound", Type: "socks5", Description: "SOCKS5 inbound", Path: "templates/snippets/inbound/socks5.yaml.tmpl", Aliases: []string{"socks"}},
	{ID: "inbound.http", Kind: "inbound", Type: "http", Description: "HTTP inbound", Path: "templates/snippets/inbound/http.yaml.tmpl"},

	{ID: "outbound.direct", Kind: "outbound", Type: "direct", Description: "Direct outbound", Path: "templates/snippets/outbound/direct.yaml.tmpl"},
	{ID: "outbound.block", Kind: "outbound", Type: "block", Description: "Block outbound", Path: "templates/snippets/outbound/block.yaml.tmpl"},
	{ID: "outbound.ref", Kind: "outbound", Type: "ref", Description: "Ref outbound", Path: "templates/snippets/outbound/ref.yaml.tmpl"},
	{ID: "outbound.shadowsocks", Kind: "outbound", Type: "shadowsocks", Description: "Shadowsocks outbound", Path: "templates/snippets/outbound/shadowsocks.yaml.tmpl", Aliases: []string{"ss", "shadowsocks22", "shadowsocks2022"}},
	{ID: "outbound.vmess-raw", Kind: "outbound", Type: "vmess-raw", Description: "VMess raw outbound", Path: "templates/snippets/outbound/vmess-raw.yaml.tmpl"},
	{ID: "outbound.vmess-websocket", Kind: "outbound", Type: "vmess-websocket", Description: "VMess WebSocket outbound", Path: "templates/snippets/outbound/vmess-websocket.yaml.tmpl", Aliases: []string{"vmess"}},
	{ID: "outbound.vmess-grpc", Kind: "outbound", Type: "vmess-grpc", Description: "VMess gRPC outbound", Path: "templates/snippets/outbound/vmess-grpc.yaml.tmpl"},
	{ID: "outbound.vless-websocket", Kind: "outbound", Type: "vless-websocket", Description: "VLESS WebSocket outbound", Path: "templates/snippets/outbound/vless-websocket.yaml.tmpl", Aliases: []string{"vless"}},
	{ID: "outbound.vless-grpc", Kind: "outbound", Type: "vless-grpc", Description: "VLESS gRPC outbound", Path: "templates/snippets/outbound/vless-grpc.yaml.tmpl"},
	{ID: "outbound.anytls", Kind: "outbound", Type: "anytls", Description: "AnyTLS outbound", Path: "templates/snippets/outbound/anytls.yaml.tmpl"},
	{ID: "outbound.trojan", Kind: "outbound", Type: "trojan", Description: "Trojan outbound", Path: "templates/snippets/outbound/trojan.yaml.tmpl"},
	{ID: "outbound.hysteria2", Kind: "outbound", Type: "hysteria2", Description: "Hysteria2 outbound", Path: "templates/snippets/outbound/hysteria2.yaml.tmpl", Aliases: []string{"hy2"}},
	{ID: "outbound.socks5", Kind: "outbound", Type: "socks5", Description: "SOCKS5 outbound", Path: "templates/snippets/outbound/socks5.yaml.tmpl", Aliases: []string{"socks"}},
	{ID: "outbound.http", Kind: "outbound", Type: "http", Description: "HTTP outbound", Path: "templates/snippets/outbound/http.yaml.tmpl"},

	{ID: "group.selector", Kind: "group", Type: "selector", Description: "Selector group", Path: "templates/snippets/group/selector.yaml.tmpl"},
	{ID: "group.urltest", Kind: "group", Type: "urltest", Description: "URLTest group", Path: "templates/snippets/group/urltest.yaml.tmpl", Aliases: []string{"url-test"}},

	{ID: "route.all", Kind: "route", Type: "all", Description: "Route all rules", Path: "templates/snippets/route/all.yaml.tmpl"},
	{ID: "route.domain", Kind: "route", Type: "domain", Description: "Route domain rule", Path: "templates/snippets/route/domain.yaml.tmpl"},
	{ID: "route.domain_suffix", Kind: "route", Type: "domain_suffix", Description: "Route domain suffix rule", Path: "templates/snippets/route/domain_suffix.yaml.tmpl"},
	{ID: "route.domain_keyword", Kind: "route", Type: "domain_keyword", Description: "Route domain keyword rule", Path: "templates/snippets/route/domain_keyword.yaml.tmpl"},
	{ID: "route.ip_cidr", Kind: "route", Type: "ip_cidr", Description: "Route IP CIDR rule", Path: "templates/snippets/route/ip_cidr.yaml.tmpl"},
	{ID: "route.geoip", Kind: "route", Type: "geoip", Description: "Route geoip rule", Path: "templates/snippets/route/geoip.yaml.tmpl"},
	{ID: "route.geosite", Kind: "route", Type: "geosite", Description: "Route geosite rule", Path: "templates/snippets/route/geosite.yaml.tmpl"},

	{ID: "transport.http", Kind: "transport", Type: "http", Description: "HTTP transport", Path: "templates/snippets/transport/http.yaml.tmpl"},
	{ID: "transport.ws", Kind: "transport", Type: "ws", Description: "WebSocket transport", Path: "templates/snippets/transport/ws.yaml.tmpl"},
	{ID: "transport.quic", Kind: "transport", Type: "quic", Description: "QUIC transport", Path: "templates/snippets/transport/quic.yaml.tmpl"},
	{ID: "transport.grpc", Kind: "transport", Type: "grpc", Description: "gRPC transport", Path: "templates/snippets/transport/grpc.yaml.tmpl"},
	{ID: "transport.httpupgrade", Kind: "transport", Type: "httpupgrade", Description: "HTTPUpgrade transport", Path: "templates/snippets/transport/httpupgrade.yaml.tmpl"},
}

// DefaultContext 返回 example 和协议参考模板使用的默认上下文。
func DefaultContext() Context {
	return Context{
		InstanceName:           "edge-us",
		Role:                   "edge",
		ShowExampleFields:      true,
		IncludeProxyAuth:       true,
		SubscriptionServer:     "proxy.example.com",
		VmessInboundName:       "vmess-main",
		VmessInboundPort:       24100,
		VmessUUID:              "11111111-1111-4111-8111-111111111111",
		VmessRemark:            "US VMess",
		VmessUserTag:           "edge-us-vmess-main",
		VlessUUID:              "33333333-3333-4333-8333-333333333333",
		AnyTLSPassword:         "change-me",
		ShadowsocksInboundName: "ss-main",
		ShadowsocksInboundPort: 24200,
		ShadowsocksPassword:    "change-me-32-byte-key",
		ShadowsocksRemark:      "US Shadowsocks",
		LocalSocksName:         "local-socks",
		LocalSocksPort:         17000,
		LocalHTTPName:          "local-http",
		LocalHTTPPort:          18000,
		VmessOutboundName:      "vmess-upstream",
		VmessOutboundUUID:      "22222222-2222-4222-8222-222222222222",
		GroupName:              "auto",
		GroupOutbounds:         "edge-us-local-socks",
		RefOutboundName:        "edge-us-local-socks",
		Ref:                    "edge-us.local-socks",
		RefMembers: []RefMember{
			{OutboundName: "edge-us-local-socks", RefOutboundName: "edge-us-local-socks", Ref: "edge-us.local-socks"},
		},
	}
}

// RenderExample 渲染指定 kind 和 type 的 example YAML 片段。
func RenderExample(kind string, typeName string, context Context) (string, error) {
	kind = normalizeTerm(kind)
	typeName = normalizeTerm(typeName)
	if typeName == "" || typeName == "all" {
		if kind == "route" {
			return RenderSnippet(kind, "all", context)
		}
		definitions := definitionsByKind(kind)
		if len(definitions) == 0 || kind == "transport" {
			return "", fmt.Errorf("unsupported example kind %q", kind)
		}
		parts := make([]string, 0, len(definitions))
		for _, definition := range definitions {
			content, err := renderDefinition(definition, context)
			if err != nil {
				return "", err
			}
			parts = append(parts, content)
		}
		return strings.Join(parts, "\n"), nil
	}
	return RenderSnippet(kind, typeName, context)
}

// RenderSnippet 渲染单个共享 YAML 片段。
func RenderSnippet(kind string, typeName string, context Context) (string, error) {
	definition, ok := lookupDefinition(kind, typeName)
	if !ok {
		return "", fmt.Errorf("unsupported %s example TYPE %q; supported values: %s", kind, typeName, strings.Join(SupportedTypes(kind), ", "))
	}
	return renderDefinition(definition, context)
}

// RenderInstance 渲染 add 使用的完整 instance 模板。
func RenderInstance(templateName string, context Context) ([]byte, error) {
	templateName = normalizeInstanceTemplate(templateName)
	if templateName == "" {
		return nil, fmt.Errorf("unsupported instance template %q", templateName)
	}
	data, err := renderTemplateFile("templates/instance/"+templateName+".yaml.tmpl", "instance."+templateName, context)
	if err != nil {
		return nil, err
	}
	return []byte(data), nil
}

// RenderProtocolReferenceComment 渲染 add 文件尾部的协议参考注释。
func RenderProtocolReferenceComment(context Context) (string, error) {
	var builder strings.Builder
	builder.WriteString("协议模板参考（默认全部注释，不参与加载；启用时复制到正文 inbounds/outbounds 并去掉前缀 #）。\n")
	builder.WriteString("inbounds:\n")
	if err := appendKindSnippets(&builder, "inbound", 2, context); err != nil {
		return "", err
	}
	builder.WriteString("transport_examples: # VMess/VLESS 可选 transport；复制其中一个 transport 块到对应 inbound/outbound。\n")
	for _, definition := range definitionsByKind("transport") {
		fmt.Fprintf(&builder, "  %s:\n", definition.Type)
		content, err := renderDefinition(definition, context)
		if err != nil {
			return "", err
		}
		builder.WriteString(indentSnippet(content, 4))
	}
	builder.WriteString("outbounds:\n")
	if err := appendKindSnippets(&builder, "outbound", 2, context); err != nil {
		return "", err
	}
	builder.WriteString("groups:\n")
	if err := appendKindSnippets(&builder, "group", 2, context); err != nil {
		return "", err
	}
	return CommentBlock(builder.String()), nil
}

// CommentBlock 把 YAML 模板块转换为可追加到配置文件中的注释块。
func CommentBlock(text string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	var builder strings.Builder
	for _, line := range lines {
		if line == "" {
			builder.WriteString("#\n")
			continue
		}
		builder.WriteString("# ")
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String()
}

// SupportedTypes 返回指定 example kind 支持的 TYPE 列表。
func SupportedTypes(kind string) []string {
	kind = normalizeTerm(kind)
	seen := map[string]struct{}{}
	items := make([]string, 0)
	for _, definition := range definitionsByKind(kind) {
		for _, item := range append([]string{definition.Type}, definition.Aliases...) {
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			items = append(items, item)
		}
	}
	if kind != "route" {
		items = append(items, "all")
	}
	return items
}

// RenderInstanceExample 渲染 example instance TYPE 使用的完整实例示例。
func RenderInstanceExample(typeName string, context Context) (string, error) {
	templateName := normalizeInstanceTemplate(typeName)
	if templateName == "" {
		return "", fmt.Errorf("unsupported instance example TYPE %q; supported values: edge, relay, urltest", typeName)
	}
	if templateName == "urltest" && len(context.RefMembers) == 1 {
		context.RefMembers = append(context.RefMembers, RefMember{OutboundName: "relay-us-local-socks", RefOutboundName: "relay-us-local-socks", Ref: "relay-us.local-socks"})
		context.GroupOutbounds = "edge-us-local-socks, relay-us-local-socks"
	}
	data, err := RenderInstance(templateName, context)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// normalizeInstanceTemplate 兼容空模板名和少量历史拼写。
func normalizeInstanceTemplate(templateName string) string {
	switch normalizeTerm(templateName) {
	case "", "edge":
		return "edge"
	case "relay":
		return "relay"
	case "urltest", "url-test":
		return "urltest"
	default:
		return ""
	}
}

func appendKindSnippets(builder *strings.Builder, kind string, indent int, context Context) error {
	for _, definition := range definitionsByKind(kind) {
		content, err := renderDefinition(definition, context)
		if err != nil {
			return err
		}
		builder.WriteString(indentSnippet(content, indent))
	}
	return nil
}

func renderDefinition(definition snippetDefinition, context Context) (string, error) {
	return renderTemplateFile(definition.Path, definition.ID, context)
}

func renderTemplateFile(path string, name string, data any) (string, error) {
	source, err := templateFiles.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("template could not be read: %s (%w)", name, err)
	}
	funcs := template.FuncMap{
		"snippet": func(id string, indent int) (string, error) {
			definition, ok := lookupDefinitionByID(id)
			if !ok {
				return "", fmt.Errorf("unknown template snippet: %s", id)
			}
			content, err := renderDefinition(definition, data.(Context))
			if err != nil {
				return "", err
			}
			return indentSnippet(content, indent), nil
		},
		"snippetWith": func(id string, snippetData any, indent int) (string, error) {
			definition, ok := lookupDefinitionByID(id)
			if !ok {
				return "", fmt.Errorf("unknown template snippet: %s", id)
			}
			content, err := renderTemplateFile(definition.Path, definition.ID, snippetData)
			if err != nil {
				return "", err
			}
			return indentSnippet(content, indent), nil
		},
	}
	tpl, err := template.New(name).Option("missingkey=error").Funcs(funcs).Parse(string(source))
	if err != nil {
		return "", fmt.Errorf("template could not be parsed: %s (%w)", name, err)
	}
	var buffer bytes.Buffer
	if err := tpl.Execute(&buffer, data); err != nil {
		return "", fmt.Errorf("template could not be rendered: %s (%w)", name, err)
	}
	return buffer.String(), nil
}

func lookupDefinition(kind string, typeName string) (snippetDefinition, bool) {
	kind = normalizeTerm(kind)
	typeName = normalizeTerm(typeName)
	for _, definition := range snippetDefinitions {
		if definition.Kind != kind {
			continue
		}
		if definition.Type == typeName {
			return definition, true
		}
		for _, alias := range definition.Aliases {
			if alias == typeName {
				return definition, true
			}
		}
	}
	return snippetDefinition{}, false
}

func lookupDefinitionByID(id string) (snippetDefinition, bool) {
	for _, definition := range snippetDefinitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return snippetDefinition{}, false
}

func definitionsByKind(kind string) []snippetDefinition {
	kind = normalizeTerm(kind)
	definitions := make([]snippetDefinition, 0)
	for _, definition := range snippetDefinitions {
		if definition.Kind == kind {
			definitions = append(definitions, definition)
		}
	}
	return definitions
}

func normalizeTerm(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "inbounds":
		return "inbound"
	case "outbounds":
		return "outbound"
	case "groups":
		return "group"
	case "url_test":
		return "urltest"
	case "url-test":
		return "urltest"
	case "domain-suffix":
		return "domain_suffix"
	case "domain-keyword":
		return "domain_keyword"
	case "ip-cidr":
		return "ip_cidr"
	default:
		return value
	}
}

func indentSnippet(content string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	lines := strings.SplitAfter(content, "\n")
	var builder strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.TrimSpace(line) == "" {
			builder.WriteString(line)
			continue
		}
		builder.WriteString(prefix)
		builder.WriteString(line)
	}
	return builder.String()
}
