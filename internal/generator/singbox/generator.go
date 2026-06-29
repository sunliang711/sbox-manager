package singbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// Generate 将领域模型转换为稳定的 sing-box JSON 字节。
func Generate(global domain.GlobalConfig, instance domain.Instance) ([]byte, error) {
	config, err := BuildConfig(global, instance)
	if err != nil {
		return nil, err
	}
	return MarshalStable(config)
}

// BuildConfig 将领域模型转换为 sing-box 内部结构，不执行任何文件或外部命令操作。
func BuildConfig(global domain.GlobalConfig, instance domain.Instance) (Config, error) {
	domain.ApplyInstanceDefaults(&instance)
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
		converted, err := convertOutbound(outbound)
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
	if inbound.UDP {
		udp := inbound.UDP
		result.UDP = &udp
	}
	if inbound.TLS.Enabled || inbound.Type == "anytls" {
		result.TLS = &TLS{Enabled: true}
	}
	if inboundSupportsTransport(inbound.Type) {
		result.Transport = convertTransport(inbound.Transport)
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
			result.Users = append(result.Users, InboundUser{
				Name:     user.Name,
				Password: user.Password,
				Method:   method,
			})
		}
		if len(result.Users) == 1 {
			result.Password = result.Users[0].Password
			if result.Method == "" {
				result.Method = result.Users[0].Method
			}
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
		return Inbound{}, fmt.Errorf("不支持的 inbound type %q", inbound.Type)
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
	if outbound.TLS.Enabled || outbound.Type == "anytls" {
		result.TLS = &TLS{Enabled: true}
	}
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
		return Outbound{}, fmt.Errorf("不支持的 outbound type %q", outbound.Type)
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
		return Outbound{}, fmt.Errorf("不支持的 group type %q", group.Type)
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
	} else if transport.Host != "" {
		result.Host = transport.Host
	}
	return result
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
