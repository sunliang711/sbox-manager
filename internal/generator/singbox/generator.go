package singbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// Generate 将领域模型转换为稳定的 sing-box JSON 字节。
func Generate(global domain.GlobalConfig, instance domain.Instance) ([]byte, error) {
	config, err := BuildConfigWithInstances(global, []domain.Instance{instance}, instance)
	if err != nil {
		return nil, err
	}
	return MarshalStable(config)
}

// GenerateWithInstances 使用完整 instance 集合解析 ref outbound 并生成 sing-box JSON 字节。
func GenerateWithInstances(global domain.GlobalConfig, instances []domain.Instance, instance domain.Instance) ([]byte, error) {
	config, err := BuildConfigWithInstances(global, instances, instance)
	if err != nil {
		return nil, err
	}
	return MarshalStable(config)
}

// BuildConfig 将领域模型转换为 sing-box 内部结构，不执行任何文件或外部命令操作。
func BuildConfig(global domain.GlobalConfig, instance domain.Instance) (Config, error) {
	return BuildConfigWithInstances(global, []domain.Instance{instance}, instance)
}

// BuildConfigWithInstances 将领域模型转换为 sing-box 内部结构，并可解析跨 instance ref outbound。
func BuildConfigWithInstances(global domain.GlobalConfig, instances []domain.Instance, instance domain.Instance) (Config, error) {
	domain.ApplyInstanceDefaults(&instance)
	instanceIndex := indexInstances(instances)
	config := Config{
		Log: Log{
			Level: global.Defaults.LogLevel,
		},
		DNS: DNS{
			Servers: []DNSServer{
				{
					Type: "local",
					Tag:  "local",
				},
			},
		},
		Inbounds:  make([]Inbound, 0, len(instance.Inbounds)),
		Outbounds: make([]Outbound, 0, len(instance.Outbounds)+len(instance.Groups)),
		Route: Route{
			Final: instance.Route.Default,
		},
	}

	for _, inbound := range instance.Inbounds {
		converted, err := convertInbound(inbound)
		if err != nil {
			return Config{}, err
		}
		config.Inbounds = append(config.Inbounds, converted)
	}
	for _, outbound := range instance.Outbounds {
		resolved, err := resolveOutboundRef(instance.Name, outbound, instanceIndex)
		if err != nil {
			return Config{}, err
		}
		converted, err := convertOutbound(resolved)
		if err != nil {
			return Config{}, err
		}
		config.Outbounds = append(config.Outbounds, converted)
	}
	for _, group := range instance.Groups {
		converted, err := convertGroup(group)
		if err != nil {
			return Config{}, err
		}
		config.Outbounds = append(config.Outbounds, converted)
	}
	for _, rule := range instance.Route.Rules {
		config.Route.Rules = append(config.Route.Rules, convertRouteRule(rule))
	}
	if instance.API.Enabled {
		config.Experimental = buildExperimental(instance)
	}
	return config, nil
}

// indexInstances 为 ref outbound 构建按 instance 名称索引的配置集合。
func indexInstances(instances []domain.Instance) map[string]domain.Instance {
	index := make(map[string]domain.Instance, len(instances))
	for _, instance := range instances {
		domain.ApplyInstanceDefaults(&instance)
		index[instance.Name] = instance
	}
	return index
}

// resolveOutboundRef 将项目内 ref outbound 解析为 sing-box 可直接使用的 socks/http outbound。
func resolveOutboundRef(currentInstance string, outbound domain.Outbound, instances map[string]domain.Instance) (domain.Outbound, error) {
	if outbound.Type != "ref" {
		return outbound, nil
	}
	parsedInstanceName, _, ok := parseOutboundRef(outbound.Ref)
	if !ok {
		return domain.Outbound{}, fmt.Errorf("ref outbound %q must use <instance>.<inbound> format", outbound.Name)
	}
	targetInstanceName, targetInboundName, targetInstance, exists := resolveOutboundRefTarget(outbound.Ref, instances)
	if !exists {
		return domain.Outbound{}, fmt.Errorf("ref outbound %q references missing instance %q", outbound.Name, parsedInstanceName)
	}
	if targetInstanceName == currentInstance {
		return domain.Outbound{}, fmt.Errorf("ref outbound %q cannot reference the current instance", outbound.Name)
	}
	for _, inbound := range targetInstance.Inbounds {
		if inbound.Name != targetInboundName {
			continue
		}
		if inbound.Type != "socks5" && inbound.Type != "http" {
			return domain.Outbound{}, fmt.Errorf("ref outbound %q can only reference socks5/http inbound", outbound.Name)
		}
		return domain.Outbound{
			Name:   outbound.Name,
			Type:   inbound.Type,
			Server: normalizeRefListen(inbound.Listen),
			Port:   inbound.Port,
			Auth:   inbound.Auth,
		}, nil
	}
	return domain.Outbound{}, fmt.Errorf("ref outbound %q references missing inbound %q", outbound.Name, outbound.Ref)
}

// resolveOutboundRefTarget 根据已有 instance 名称解析 ref，支持 instance 名称包含点号。
func resolveOutboundRefTarget(ref string, instances map[string]domain.Instance) (string, string, domain.Instance, bool) {
	trimmed := strings.TrimSpace(ref)
	matchedName := ""
	matchedInbound := ""
	matchedInstance := domain.Instance{}
	for instanceName, instance := range instances {
		prefix := instanceName + "."
		if !strings.HasPrefix(trimmed, prefix) || len(instanceName) <= len(matchedName) {
			continue
		}
		inboundName := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if inboundName == "" {
			continue
		}
		matchedName = instanceName
		matchedInbound = inboundName
		matchedInstance = instance
	}
	if matchedName == "" {
		return "", "", domain.Instance{}, false
	}
	return matchedName, matchedInbound, matchedInstance, true
}

// parseOutboundRef 解析 `<instance>.<inbound>` 格式引用。
func parseOutboundRef(ref string) (string, string, bool) {
	trimmed := strings.TrimSpace(ref)
	separator := strings.LastIndex(trimmed, ".")
	if separator <= 0 || separator == len(trimmed)-1 {
		return "", "", false
	}
	return strings.TrimSpace(trimmed[:separator]), strings.TrimSpace(trimmed[separator+1:]), true
}

// normalizeRefListen 将通配监听地址转换为本机可连接地址。
func normalizeRefListen(listen string) string {
	listen = strings.TrimSpace(listen)
	ip := net.ParseIP(listen)
	if ip == nil || !ip.IsUnspecified() {
		return listen
	}
	if ip.To4() != nil {
		return "127.0.0.1"
	}
	return "::1"
}

// MarshalStable 使用固定缩进和关闭 HTML 转义的 JSON 输出，保证同输入字节级稳定。
func MarshalStable(value interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// convertInbound 将单个领域 inbound 转换为 sing-box inbound。
func convertInbound(inbound domain.Inbound) (Inbound, error) {
	result := Inbound{
		Type:       singBoxInboundType(inbound.Type),
		Tag:        inbound.Tag,
		Listen:     inbound.Listen,
		ListenPort: inbound.Port,
	}
	result.TLS = convertTLS(inbound.TLS, inbound.Type == "anytls")
	if inboundSupportsTransport(inbound.Type) {
		result.Transport = convertInboundTransport(inbound.Transport)
	}

	switch inbound.Type {
	case "vmess":
		result.Users = make([]InboundUser, 0, len(inbound.Users))
		for _, user := range inbound.Users {
			result.Users = append(result.Users, InboundUser{
				Name:    user.Name,
				UUID:    user.UUID,
				AlterID: user.AlterID,
			})
		}
	case "vless":
		result.Users = make([]InboundUser, 0, len(inbound.Users))
		for _, user := range inbound.Users {
			result.Users = append(result.Users, InboundUser{
				Name: user.Name,
				UUID: user.UUID,
				Flow: user.Flow,
			})
		}
	case "anytls":
		result.Users = make([]InboundUser, 0, len(inbound.Users))
		for _, user := range inbound.Users {
			result.Users = append(result.Users, InboundUser{
				Name:     user.Name,
				Password: user.Password,
			})
		}
	case "shadowsocks":
		result.Method = inbound.Method
		result.Users = make([]InboundUser, 0, len(inbound.Users))
		for _, user := range inbound.Users {
			method := user.Method
			if method == "" {
				method = inbound.Method
			}
			if result.Method == "" {
				result.Method = method
			}
			result.Users = append(result.Users, InboundUser{
				Name:     user.Name,
				Password: user.Password,
			})
		}
		if len(result.Users) == 1 {
			result.Password = result.Users[0].Password
		}
	case "socks5", "http":
		if inbound.Auth.Type == "password" {
			result.Users = []InboundUser{
				{
					Username: inbound.Auth.Username,
					Password: inbound.Auth.Password,
				},
			}
		}
	default:
		return Inbound{}, fmt.Errorf("unsupported inbound type %q", inbound.Type)
	}
	return result, nil
}

// convertOutbound 将单个领域 outbound 转换为 sing-box outbound。
func convertOutbound(outbound domain.Outbound) (Outbound, error) {
	result := Outbound{
		Type:       singBoxOutboundType(outbound.Type),
		Tag:        outbound.Name,
		Server:     outbound.Server,
		ServerPort: outbound.Port,
		UUID:       outbound.UUID,
		Password:   outbound.Password,
		Method:     outbound.Method,
	}
	if outbound.Type == "vmess" {
		result.Network = outbound.Network
		result.Security = outbound.Security
		result.AlterID = outbound.AlterID
	}
	if outbound.Type == "vless" {
		result.Flow = outbound.Flow
	}
	result.TLS = convertTLS(outbound.TLS, outbound.Type == "anytls")
	if outboundSupportsTransport(outbound.Type) {
		result.Transport = convertTransport(outbound.Transport)
	}
	if outbound.Auth.Type == "password" {
		result.Username = outbound.Auth.Username
		result.Password = outbound.Auth.Password
	}

	switch outbound.Type {
	case "direct", "block", "shadowsocks", "vmess", "vless", "anytls", "trojan", "hysteria2", "socks5", "http":
		return result, nil
	default:
		return Outbound{}, fmt.Errorf("unsupported outbound type %q", outbound.Type)
	}
}

// convertGroup 将领域 group 转换为 sing-box outbound group。
func convertGroup(group domain.Group) (Outbound, error) {
	result := Outbound{
		Type:      group.Type,
		Tag:       group.Name,
		Outbounds: append([]string(nil), group.Outbounds...),
	}
	switch group.Type {
	case "selector":
		return result, nil
	case "urltest":
		result.URL = group.URL
		result.Interval = strconv.Itoa(group.Interval) + "s"
		result.Tolerance = group.Tolerance
		return result, nil
	default:
		return Outbound{}, fmt.Errorf("unsupported group type %q", group.Type)
	}
}

// convertRouteRule 将领域路由规则转换为 sing-box 路由规则。
func convertRouteRule(rule domain.RouteRule) RouteRule {
	converted := RouteRule{
		Outbound: rule.Outbound,
	}
	values := append([]string(nil), rule.Values...)
	switch rule.Type {
	case "domain":
		converted.Domain = values
	case "domain_suffix":
		converted.DomainSuffix = values
	case "domain_keyword":
		converted.DomainKeyword = values
	case "ip_cidr":
		converted.IPCIDR = values
	case "geoip":
		converted.GeoIP = values
	case "geosite":
		converted.Geosite = values
	}
	return converted
}

// convertInboundTransport 将领域 transport 转换为 sing-box inbound 传输配置。
func convertInboundTransport(transport domain.TransportConfig) *Transport {
	result := convertTransport(transport)
	if result == nil {
		return nil
	}
	result.Host = nil
	return result
}

// convertTransport 将领域 transport 转换为 sing-box V2Ray transport 配置。
func convertTransport(transport domain.TransportConfig) *Transport {
	if transport.Type == "" {
		return nil
	}
	result := &Transport{
		Type:                transport.Type,
		Path:                transport.Path,
		Method:              transport.Method,
		Headers:             transport.Headers,
		IdleTimeout:         transport.IdleTimeout,
		PingTimeout:         transport.PingTimeout,
		MaxEarlyData:        transport.MaxEarlyData,
		EarlyDataHeaderName: transport.EarlyDataHeaderName,
		ServiceName:         transport.ServiceName,
		PermitWithoutStream: transport.PermitWithoutStream,
	}
	if transport.Type == "http" && len(transport.Hosts) > 0 {
		result.Host = append([]string(nil), transport.Hosts...)
	} else if transport.Type == "httpupgrade" && transport.Host != "" {
		result.Host = transport.Host
	}
	return result
}

// convertTLS 将领域 TLS 配置转换为 sing-box TLS 片段。
func convertTLS(tls domain.TLSConfig, forceEnabled bool) *TLS {
	if !tls.Enabled && !forceEnabled {
		return nil
	}
	return &TLS{
		Enabled:         tls.Enabled || forceEnabled,
		ServerName:      tls.ServerName,
		Insecure:        tls.Insecure,
		ALPN:            append([]string(nil), tls.ALPN...),
		CertificatePath: tls.CertificatePath,
		KeyPath:         tls.KeyPath,
	}
}

// inboundSupportsTransport 判断 inbound 协议是否支持 V2Ray transport。
func inboundSupportsTransport(inboundType string) bool {
	return inboundType == "vmess" || inboundType == "vless"
}

// outboundSupportsTransport 判断 outbound 协议是否支持 V2Ray transport。
func outboundSupportsTransport(outboundType string) bool {
	return outboundType == "vmess" || outboundType == "vless"
}

// buildExperimental 根据 instance API 配置生成 sing-box experimental 片段。
func buildExperimental(instance domain.Instance) *Experimental {
	stats := V2RayStats{
		Enabled: true,
	}
	for _, inbound := range instance.Inbounds {
		stats.Inbounds = append(stats.Inbounds, inbound.Tag)
		for _, user := range inbound.Users {
			if user.Name != "" {
				stats.Users = append(stats.Users, user.Name)
			}
		}
	}
	for _, outbound := range instance.Outbounds {
		stats.Outbounds = append(stats.Outbounds, outbound.Name)
	}
	return &Experimental{
		V2RayAPI: &V2RayAPI{
			Listen: instance.API.Listen,
			Stats:  stats,
		},
	}
}

// singBoxInboundType 将领域协议名转换为 sing-box inbound 类型名。
func singBoxInboundType(inboundType string) string {
	if inboundType == "socks5" {
		return "socks"
	}
	return inboundType
}

// singBoxOutboundType 将领域协议名转换为 sing-box outbound 类型名。
func singBoxOutboundType(outboundType string) string {
	if outboundType == "socks5" {
		return "socks"
	}
	return outboundType
}
