package singbox

import (
	"fmt"
	"strings"
	"time"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

const (
	subscriptionInputSchema  = "sbox.subscription-input"
	subscriptionInputVersion = 1
)

// BuildSubscriptionInputs 根据启用订阅的 inbound 生成订阅 input 列表。
func BuildSubscriptionInputs(global domain.GlobalConfig, instances []domain.Instance, generatedAt time.Time) ([]domain.SubscriptionInput, error) {
	inputs := make([]domain.SubscriptionInput, 0, len(instances))
	for _, instance := range instances {
		input, err := BuildSubscriptionInput(global, instance, generatedAt)
		if err != nil {
			return nil, err
		}
		if len(input.Nodes) > 0 {
			inputs = append(inputs, input)
		}
	}
	return inputs, nil
}

// BuildSubscriptionInput 根据单个 instance 生成订阅 input。
func BuildSubscriptionInput(global domain.GlobalConfig, instance domain.Instance, generatedAt time.Time) (domain.SubscriptionInput, error) {
	input := domain.SubscriptionInput{
		InputSchema:  subscriptionInputSchema,
		InputVersion: subscriptionInputVersion,
		Source:       instance.Name,
		GeneratedAt:  generatedAt.Format(time.RFC3339),
		ExternalHost: global.ExternalHost,
		Nodes:        []domain.SubscriptionNode{},
	}
	for _, inbound := range instance.Inbounds {
		if !inbound.Subscription.Enabled {
			continue
		}
		node, err := buildSubscriptionNode(global, instance, inbound)
		if err != nil {
			return domain.SubscriptionInput{}, err
		}
		input.Nodes = append(input.Nodes, node)
	}
	return input, nil
}

// RenderUserConfig 根据用户和目标格式输出最小可用订阅配置。
func RenderUserConfig(format string, user string, inputs []domain.SubscriptionInput) ([]byte, error) {
	nodes := filterNodesByUser(user, inputs)
	if len(nodes) == 0 {
		return nil, fmt.Errorf("用户 %q 没有可导出的订阅节点", user)
	}
	switch format {
	case "sing-box":
		return renderSingBoxUserConfig(nodes)
	case "clash", "premium-clash":
		return renderClashUserConfig(nodes)
	case "surge":
		return renderSurgeUserConfig(nodes)
	default:
		return nil, fmt.Errorf("不支持的导出格式 %q", format)
	}
}

// buildSubscriptionNode 将一个启用订阅的 inbound 转换为订阅节点。
func buildSubscriptionNode(global domain.GlobalConfig, instance domain.Instance, inbound domain.Inbound) (domain.SubscriptionNode, error) {
	server := inbound.Subscription.Server
	if server == "" {
		server = global.ExternalHost
	}
	if server == "" {
		return domain.SubscriptionNode{}, fmt.Errorf("instance %s inbound %s 缺少 external_host", instance.Name, inbound.Name)
	}

	user := findInboundUser(inbound, inbound.Subscription.User)
	remark := inbound.Subscription.Remark
	if remark == "" {
		remark = user.Remark
	}
	if remark == "" {
		remark = inbound.Name
	}
	tag := user.Tag
	if tag == "" {
		tag = instance.Name + "-" + inbound.Tag
	}

	node := domain.SubscriptionNode{
		ID:       instance.Name + ":" + inbound.Subscription.User + ":" + inbound.Name,
		User:     inbound.Subscription.User,
		Protocol: inbound.Type,
		Server:   server,
		Port:     inbound.Port,
		Tag:      tag,
		Remark:   remark,
		Region:   inbound.Subscription.Region,
		UDP:      inbound.UDP,
	}
	switch inbound.Type {
	case "vmess":
		node.UUID = user.UUID
		node.Network = "tcp"
	case "shadowsocks":
		node.Method = user.Method
		if node.Method == "" {
			node.Method = inbound.Method
		}
		node.Password = user.Password
	case "socks5", "http":
		node.Auth = inbound.Auth
	default:
		return domain.SubscriptionNode{}, fmt.Errorf("不支持的订阅协议 %q", inbound.Type)
	}
	return node, nil
}

// findInboundUser 按名称查找 inbound 用户，未找到时返回空用户用于 socks/http 等协议。
func findInboundUser(inbound domain.Inbound, name string) domain.InboundUser {
	for _, user := range inbound.Users {
		if user.Name == name {
			return user
		}
	}
	return domain.InboundUser{}
}

// filterNodesByUser 从多个订阅 input 中筛选指定用户节点。
func filterNodesByUser(user string, inputs []domain.SubscriptionInput) []domain.SubscriptionNode {
	var nodes []domain.SubscriptionNode
	for _, input := range inputs {
		for _, node := range input.Nodes {
			if node.User == user {
				nodes = append(nodes, node)
			}
		}
	}
	return nodes
}

// renderSingBoxUserConfig 输出 sing-box 客户端可用的基础 outbounds 配置。
func renderSingBoxUserConfig(nodes []domain.SubscriptionNode) ([]byte, error) {
	outbounds := make([]Outbound, 0, len(nodes)+1)
	for _, node := range nodes {
		outbounds = append(outbounds, outboundFromSubscriptionNode(node))
	}
	if len(outbounds) > 0 {
		outbounds = append(outbounds, Outbound{
			Type:      "selector",
			Tag:       "proxy",
			Outbounds: outboundTags(outbounds),
		})
	}
	return MarshalStable(struct {
		Outbounds []Outbound `json:"outbounds"`
		Route     Route      `json:"route"`
	}{
		Outbounds: outbounds,
		Route: Route{
			Final: "proxy",
		},
	})
}

// renderClashUserConfig 输出 Clash/Premium Clash 的基础 YAML 文本。
func renderClashUserConfig(nodes []domain.SubscriptionNode) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("proxies:\n")
	for _, node := range nodes {
		switch node.Protocol {
		case "vmess":
			fmt.Fprintf(&builder, "  - name: %q\n    type: vmess\n    server: %s\n    port: %d\n    uuid: %s\n    alterId: 0\n    cipher: auto\n    network: %s\n", node.Remark, node.Server, node.Port, node.UUID, valueOrDefault(node.Network, "tcp"))
		case "shadowsocks":
			fmt.Fprintf(&builder, "  - name: %q\n    type: ss\n    server: %s\n    port: %d\n    cipher: %s\n    password: %q\n", node.Remark, node.Server, node.Port, node.Method, node.Password)
		case "socks5":
			fmt.Fprintf(&builder, "  - name: %q\n    type: socks5\n    server: %s\n    port: %d\n", node.Remark, node.Server, node.Port)
			appendClashAuth(&builder, node.Auth)
		case "http":
			fmt.Fprintf(&builder, "  - name: %q\n    type: http\n    server: %s\n    port: %d\n", node.Remark, node.Server, node.Port)
			appendClashAuth(&builder, node.Auth)
		}
	}
	builder.WriteString("proxy-groups:\n")
	builder.WriteString("  - name: proxy\n    type: select\n    proxies:\n")
	for _, node := range nodes {
		fmt.Fprintf(&builder, "      - %q\n", node.Remark)
	}
	builder.WriteString("rules:\n  - MATCH,proxy\n")
	return []byte(builder.String()), nil
}

// renderSurgeUserConfig 输出 Surge 的基础文本配置。
func renderSurgeUserConfig(nodes []domain.SubscriptionNode) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("[Proxy]\n")
	for _, node := range nodes {
		switch node.Protocol {
		case "vmess":
			fmt.Fprintf(&builder, "%s = vmess, %s, %d, username=%s\n", node.Remark, node.Server, node.Port, node.UUID)
		case "shadowsocks":
			fmt.Fprintf(&builder, "%s = ss, %s, %d, encrypt-method=%s, password=%s\n", node.Remark, node.Server, node.Port, node.Method, node.Password)
		case "socks5":
			fmt.Fprintf(&builder, "%s = socks5, %s, %d%s\n", node.Remark, node.Server, node.Port, surgeAuthSuffix(node.Auth))
		case "http":
			fmt.Fprintf(&builder, "%s = http, %s, %d%s\n", node.Remark, node.Server, node.Port, surgeAuthSuffix(node.Auth))
		}
	}
	builder.WriteString("\n[Proxy Group]\nproxy = select")
	for _, node := range nodes {
		fmt.Fprintf(&builder, ", %s", node.Remark)
	}
	builder.WriteString("\n\n[Rule]\nFINAL,proxy\n")
	return []byte(builder.String()), nil
}

// outboundFromSubscriptionNode 将订阅节点转换为 sing-box outbound。
func outboundFromSubscriptionNode(node domain.SubscriptionNode) Outbound {
	outbound := Outbound{
		Type:       singBoxOutboundType(node.Protocol),
		Tag:        node.Tag,
		Server:     node.Server,
		ServerPort: node.Port,
		UUID:       node.UUID,
		Method:     node.Method,
		Password:   node.Password,
		Network:    node.Network,
	}
	if node.Auth.Type == "password" {
		outbound.Username = node.Auth.Username
		outbound.Password = node.Auth.Password
	}
	return outbound
}

// outboundTags 返回 outbound tag 列表。
func outboundTags(outbounds []Outbound) []string {
	tags := make([]string, 0, len(outbounds))
	for _, outbound := range outbounds {
		tags = append(tags, outbound.Tag)
	}
	return tags
}

// appendClashAuth 追加 Clash 代理认证字段。
func appendClashAuth(builder *strings.Builder, auth domain.AuthConfig) {
	if auth.Type != "password" {
		return
	}
	fmt.Fprintf(builder, "    username: %q\n    password: %q\n", auth.Username, auth.Password)
}

// surgeAuthSuffix 生成 Surge 代理认证字段后缀。
func surgeAuthSuffix(auth domain.AuthConfig) string {
	if auth.Type != "password" {
		return ""
	}
	return fmt.Sprintf(", username=%s, password=%s", auth.Username, auth.Password)
}

// valueOrDefault 返回非空值或默认值。
func valueOrDefault(value string, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
