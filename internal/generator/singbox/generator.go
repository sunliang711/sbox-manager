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
					Tag:     "local",
					Address: "local",
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

	switch inbound.Type {
	case "vmess":
		result.Users = make([]InboundUser, 0, len(inbound.Users))
		for _, user := range inbound.Users {
			result.Users = append(result.Users, InboundUser{
				Name: user.Name,
				UUID: user.UUID,
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
		Network:    outbound.Network,
	}
	if outbound.TLS.Enabled {
		result.TLS = &TLS{Enabled: true}
	}
	if outbound.Network != "" && outbound.Network != "tcp" && outbound.Type == "vmess" {
		result.Transport = &Transport{Type: outbound.Network}
	}
	if outbound.Auth.Type == "password" {
		result.Username = outbound.Auth.Username
		result.Password = outbound.Auth.Password
	}

	switch outbound.Type {
	case "direct", "block", "shadowsocks", "vmess", "trojan", "hysteria2", "socks5", "http":
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
