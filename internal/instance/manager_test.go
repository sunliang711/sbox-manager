package instance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
	"github.com/sunliang711/sbox-manager/internal/generator/singbox"
	"github.com/sunliang711/sbox-manager/internal/subscription"
)

// TestInitWritesCommentedGlobalConfig 验证 init 生成的全局配置带字段说明且仍可加载。
func TestInitWritesCommentedGlobalConfig(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(baseDir, "config.yaml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, want := range []string{"# version:", "# external_host:", "# paths:", "# port_ranges:"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("config missing comment %q:\n%s", want, data)
		}
	}
	if _, err := config.LoadGlobalConfig(filepath.Join(baseDir, "config.yaml"), baseDir); err != nil {
		t.Fatalf("load commented config: %v", err)
	}
}

// TestAddWritesCommentedInstanceConfig 验证新增实例配置包含模板说明注释且仍可加载。
func TestAddWritesCommentedInstanceConfig(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-smoke", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge: %v", err)
	}
	path := filepath.Join(baseDir, "instances", "edge-smoke.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read instance: %v", err)
	}
	for _, want := range []string{
		"# 生效配置从下方 name 字段开始",
		"# 最少需要确认:",
		"#   - inbounds[].port:",
		"#   - inbounds[].users[].uuid/password:",
		"#   - outbounds[].server/port/uuid:",
		"#   - inbounds[].subscription.remark/region:",
		"# 常用命令:",
		"# 协议模板参考",
		"# outbounds:",
		"#     type: vmess",
		"#     type: vless",
		"#     type: anytls",
		"#     type: shadowsocks",
		"#     type: socks5",
		"#     type: http",
		"#     type: direct",
		"#     type: block",
		"#     type: ref",
		"#     type: trojan",
		"#     type: hysteria2",
		"#       server_name: vmess.example.com",
		"#       insecure: false",
		"#       alpn: [h2, http/1.1]",
		"# transport_examples:",
		"#       type: http",
		"#       type: ws",
		"#       type: quic",
		"#       type: grpc",
		"#       type: httpupgrade",
		"#       service_name: TunService",
		"# groups:",
		"#     outbounds: [edge-us-local-socks]",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("instance missing comment %q:\n%s", want, data)
		}
	}
	bodyStart := strings.Index(string(data), "\nname: edge-smoke\n")
	if bodyStart < 0 {
		t.Fatalf("instance body missing:\n%s", data)
	}
	templateStart := strings.Index(string(data), "\n# 协议模板参考")
	if templateStart < 0 {
		t.Fatalf("instance protocol template missing:\n%s", data)
	}
	if templateStart < bodyStart {
		t.Fatalf("instance body should appear before protocol template:\n%s", data)
	}
	body := string(data[bodyStart:templateStart])
	for _, want := range []string{"security: auto", "tls:", "server_name: vmess.example.com", "transport:", "type: ws", "Host: vmess.example.com"} {
		if !strings.Contains(body, want) {
			t.Fatalf("instance body missing %q:\n%s", want, body)
		}
	}
	for _, unexpected := range []string{"type: noauth", "labels: []", "users: []", "groups: []", "rules: []", "ref: \"\"", "port: 0"} {
		if strings.Contains(body, unexpected) {
			t.Fatalf("instance body should not contain %q:\n%s", unexpected, body)
		}
	}
	global, err := config.LoadGlobalConfig(filepath.Join(baseDir, "config.yaml"), baseDir)
	if err != nil {
		t.Fatalf("load global: %v", err)
	}
	if _, err := config.LoadInstance(path, *global); err != nil {
		t.Fatalf("load commented instance: %v", err)
	}
}

// TestAddTemplatesIncludeLocalProxyInbounds 验证内置模板默认附带本地 socks/http 代理入口。
func TestAddTemplatesIncludeLocalProxyInbounds(t *testing.T) {
	for _, template := range []string{"edge", "relay", "urltest"} {
		t.Run(template, func(t *testing.T) {
			baseDir := t.TempDir()
			if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
				t.Fatalf("init: %v", err)
			}
			options := AddOptions{Name: template + "-smoke", Template: template, AllocatePorts: true}
			if template == "urltest" {
				if _, err := Add(baseDir, AddOptions{Name: "edge-member", Template: "edge", AllocatePorts: true}); err != nil {
					t.Fatalf("add member edge: %v", err)
				}
				options.Members = []string{"edge-member"}
			}
			instance, err := Add(baseDir, options)
			if err != nil {
				t.Fatalf("add %s: %v", template, err)
			}

			socks := findTestInbound(instance.Inbounds, "local-socks")
			if socks == nil {
				t.Fatalf("%s template missing local-socks inbound: %+v", template, instance.Inbounds)
			}
			wantSocksPort := 17000
			wantHTTPPort := 18000
			if template == "urltest" {
				wantSocksPort = 17001
				wantHTTPPort = 18001
			}
			if socks.Type != "socks5" || socks.Listen != "127.0.0.1" || socks.Port != wantSocksPort {
				t.Fatalf("unexpected local-socks inbound: %+v", *socks)
			}
			http := findTestInbound(instance.Inbounds, "local-http")
			if http == nil {
				t.Fatalf("%s template missing local-http inbound: %+v", template, instance.Inbounds)
			}
			if http.Type != "http" || http.Listen != "127.0.0.1" || http.Port != wantHTTPPort {
				t.Fatalf("unexpected local-http inbound: %+v", *http)
			}
			primary := instance.Inbounds[0]
			if !primary.Subscription.Enabled || primary.Subscription.User != "alice" || primary.Subscription.Remark != instance.Name {
				t.Fatalf("unexpected default subscription on primary inbound: %+v", primary.Subscription)
			}
			if template != "urltest" {
				if len(instance.Outbounds) != 1 || instance.Outbounds[0].Name != "vmess-upstream" || instance.Outbounds[0].Type != "vmess" {
					t.Fatalf("unexpected default outbound: %+v", instance.Outbounds)
				}
				if instance.Route.Default != "vmess-upstream" {
					t.Fatalf("route default = %q, want vmess-upstream", instance.Route.Default)
				}
				outbound := instance.Outbounds[0]
				if !outbound.TLS.Enabled || outbound.TLS.ServerName != "vmess.example.com" || outbound.Transport.Type != "ws" || outbound.Transport.Headers["Host"] != "vmess.example.com" {
					t.Fatalf("unexpected vmess upstream detail: %+v", outbound)
				}
			}
		})
	}
}

// TestAddURLTestTemplateUsesRefMembers 验证 urltest 模板会把已有 instance 作为 ref outbound 成员。
func TestAddURLTestTemplateUsesRefMembers(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-member", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge member: %v", err)
	}
	instance, err := Add(baseDir, AddOptions{Name: "auto-smoke", Template: "urltest", Members: []string{"edge-member"}, AllocatePorts: true})
	if err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	if len(instance.Outbounds) != 1 {
		t.Fatalf("urltest outbounds = %+v", instance.Outbounds)
	}
	if instance.Outbounds[0].Type != "ref" || instance.Outbounds[0].Ref != "edge-member.local-socks" {
		t.Fatalf("unexpected ref outbound: %+v", instance.Outbounds[0])
	}
	if len(instance.Groups) != 1 || len(instance.Groups[0].Outbounds) != 1 || instance.Groups[0].Outbounds[0] != "edge-member-local-socks" {
		t.Fatalf("unexpected urltest group: %+v", instance.Groups)
	}
}

// TestAddURLTestTemplateAllowsDottedMemberName 验证 ref 解析支持带点号的 instance 名。
func TestAddURLTestTemplateAllowsDottedMemberName(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge.us", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add dotted member: %v", err)
	}
	instance, err := Add(baseDir, AddOptions{Name: "auto-smoke", Template: "urltest", Members: []string{"edge.us"}, AllocatePorts: true})
	if err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	if len(instance.Outbounds) != 1 || instance.Outbounds[0].Ref != "edge.us.local-socks" {
		t.Fatalf("unexpected ref outbound: %+v", instance.Outbounds)
	}
	if _, err := config.LoadAgentConfigSet(baseDir); err != nil {
		t.Fatalf("load config set: %v", err)
	}
}

// TestAddRefMemberAllocatesHashedNameOnCollision 验证 ref outbound 名称碰撞时追加稳定哈希。
func TestAddRefMemberAllocatesHashedNameOnCollision(t *testing.T) {
	instance := domain.Instance{
		Name: "auto-smoke",
		Groups: []domain.Group{
			{Name: "auto", Type: "urltest"},
		},
	}
	existing := []domain.Instance{
		{
			Name: "edge-us",
			Inbounds: []domain.Inbound{
				{Name: "local-socks", Type: "socks5"},
			},
		},
		{
			Name: "edge",
			Inbounds: []domain.Inbound{
				{Name: "us-local-socks", Type: "socks5"},
			},
		},
	}
	if err := addRefMemberToInstance(&instance, existing, "auto", "edge-us"); err != nil {
		t.Fatalf("add edge-us member: %v", err)
	}
	if err := addRefMemberToInstance(&instance, existing, "auto", "edge"); err != nil {
		t.Fatalf("add edge member: %v", err)
	}
	if len(instance.Outbounds) != 2 {
		t.Fatalf("outbounds = %+v", instance.Outbounds)
	}
	if instance.Outbounds[0].Name != "edge-us-local-socks" {
		t.Fatalf("first outbound name = %q", instance.Outbounds[0].Name)
	}
	if instance.Outbounds[1].Name == "edge-us-local-socks" || !strings.HasPrefix(instance.Outbounds[1].Name, "edge-us-local-socks-") {
		t.Fatalf("second outbound name should use hash suffix: %+v", instance.Outbounds[1])
	}
	if len(instance.Groups[0].Outbounds) != 2 || instance.Groups[0].Outbounds[0] == instance.Groups[0].Outbounds[1] {
		t.Fatalf("unexpected group outbounds: %+v", instance.Groups[0].Outbounds)
	}
}

// TestAddURLTestTemplateRequiresMembers 验证 urltest 模板必须显式指定已有成员。
func TestAddURLTestTemplateRequiresMembers(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "auto-smoke", Template: "urltest", AllocatePorts: true}); err == nil {
		t.Fatal("expected urltest without members to fail")
	}
}

// TestMemberCommandsUseRefMembers 验证 member 子命令按已有 instance ref 成员维护 group。
func TestMemberCommandsUseRefMembers(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-one", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge-one: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-two", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge-two: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "auto-smoke", Template: "urltest", Members: []string{"edge-one"}, AllocatePorts: true}); err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	if err := MemberAdd(baseDir, "auto-smoke", "auto", "edge-two"); err != nil {
		t.Fatalf("member add: %v", err)
	}
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("load config set: %v", err)
	}
	members, err := MemberList(set, "auto-smoke", "auto")
	if err != nil {
		t.Fatalf("member list: %v", err)
	}
	if strings.Join(members, ",") != "edge-one,edge-two" {
		t.Fatalf("members = %+v", members)
	}
	auto, ok := set.FindInstance("auto-smoke")
	if !ok {
		t.Fatal("auto instance missing")
	}
	if !hasRefOutbound(auto, "edge-two-local-socks", "edge-two.local-socks") {
		t.Fatalf("edge-two ref outbound missing: %+v", auto.Outbounds)
	}
	if err := MemberRemove(baseDir, "auto-smoke", "auto", "edge-one"); err != nil {
		t.Fatalf("member remove: %v", err)
	}
	set, err = config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("reload config set: %v", err)
	}
	members, err = MemberList(set, "auto-smoke", "auto")
	if err != nil {
		t.Fatalf("member list after remove: %v", err)
	}
	if strings.Join(members, ",") != "edge-two" {
		t.Fatalf("members after remove = %+v", members)
	}
	auto, ok = set.FindInstance("auto-smoke")
	if !ok {
		t.Fatal("auto instance missing after remove")
	}
	if hasRefOutbound(auto, "edge-one-local-socks", "edge-one.local-socks") {
		t.Fatalf("edge-one ref outbound should be removed: %+v", auto.Outbounds)
	}
	if err := MemberAdd(baseDir, "auto-smoke", "auto", "missing"); err == nil {
		t.Fatal("expected missing member add to fail")
	}
}

// TestMemberRemoveKeepsSharedRefOutbound 验证删除单个 group 成员时保留其它 group 仍引用的 ref outbound。
func TestMemberRemoveKeepsSharedRefOutbound(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-one", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge-one: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-two", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge-two: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "auto-smoke", Template: "urltest", Members: []string{"edge-one"}, AllocatePorts: true}); err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	if err := MemberAdd(baseDir, "auto-smoke", "auto", "edge-two"); err != nil {
		t.Fatalf("member add: %v", err)
	}
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("load config set: %v", err)
	}
	auto, ok := set.FindInstance("auto-smoke")
	if !ok {
		t.Fatal("auto instance missing")
	}
	auto.Groups = append(auto.Groups, domain.Group{
		Name:      "backup",
		Type:      "selector",
		Outbounds: []string{"edge-one-local-socks"},
	})
	if err := WriteInstance(set.Global, set.Instances, auto); err != nil {
		t.Fatalf("write shared group: %v", err)
	}
	if err := MemberRemove(baseDir, "auto-smoke", "auto", "edge-one"); err != nil {
		t.Fatalf("member remove: %v", err)
	}
	set, err = config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("reload config set: %v", err)
	}
	auto, ok = set.FindInstance("auto-smoke")
	if !ok {
		t.Fatal("auto instance missing after remove")
	}
	members, err := MemberList(set, "auto-smoke", "auto")
	if err != nil {
		t.Fatalf("member list: %v", err)
	}
	if strings.Join(members, ",") != "edge-two" {
		t.Fatalf("auto members = %+v", members)
	}
	if !hasRefOutbound(auto, "edge-one-local-socks", "edge-one.local-socks") {
		t.Fatalf("shared ref outbound should be kept: %+v", auto.Outbounds)
	}
	if !groupHasOutbound(auto.Groups, "backup", "edge-one-local-socks") {
		t.Fatalf("backup group should keep shared outbound: %+v", auto.Groups)
	}
}

// TestPortRangeForInboundKeepsPublicProxyInInboundRange 验证公开 socks/http 仍使用普通入口端口段。
func TestPortRangeForInboundKeepsPublicProxyInInboundRange(t *testing.T) {
	global := domain.DefaultGlobalConfig()
	tests := []struct {
		name    string
		inbound domain.Inbound
		want    domain.PortRange
	}{
		{name: "local socks", inbound: domain.Inbound{Type: "socks5", Listen: "127.0.0.1"}, want: global.PortRanges.LocalSocks},
		{name: "local http", inbound: domain.Inbound{Type: "http", Listen: "::1"}, want: global.PortRanges.LocalHTTP},
		{name: "public socks", inbound: domain.Inbound{Type: "socks5", Listen: "0.0.0.0"}, want: global.PortRanges.Inbound},
		{name: "public http", inbound: domain.Inbound{Type: "http", Listen: "203.0.113.10"}, want: global.PortRanges.Inbound},
		{name: "vmess", inbound: domain.Inbound{Type: "vmess", Listen: "0.0.0.0"}, want: global.PortRanges.Inbound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := portRangeForInbound(global, tt.inbound)
			if got != tt.want {
				t.Fatalf("port range = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestCloneAllocatesPortsWithoutMutatingSource 验证克隆重新分配端口不会污染源实例并触发自身冲突。
func TestCloneAllocatesPortsWithoutMutatingSource(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	edge, err := Add(baseDir, AddOptions{Name: "edge-smoke", Template: "edge", AllocatePorts: true})
	if err != nil {
		t.Fatalf("add edge: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "urltest-smoke", Template: "urltest", Members: []string{"edge-smoke"}, AllocatePorts: true}); err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	cloned, err := Clone(baseDir, CloneOptions{Source: "edge-smoke", Target: "edge-clone", AllocatePorts: true})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if cloned.Inbounds[0].Port == edge.Inbounds[0].Port {
		t.Fatalf("clone should allocate a new port, got %d", cloned.Inbounds[0].Port)
	}
	if cloned.Inbounds[0].Subscription.Remark != "edge-clone" {
		t.Fatalf("clone subscription remark = %q, want edge-clone", cloned.Inbounds[0].Subscription.Remark)
	}

	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("load config set: %v", err)
	}
	source, ok := set.FindInstance("edge-smoke")
	if !ok {
		t.Fatal("source instance missing")
	}
	if source.Inbounds[0].Port != edge.Inbounds[0].Port {
		t.Fatalf("source port mutated: got %d want %d", source.Inbounds[0].Port, edge.Inbounds[0].Port)
	}
	generatedAt := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	inputs, err := singbox.BuildSubscriptionInputs(set.Global, set.Instances, generatedAt)
	if err != nil {
		t.Fatalf("build subscription inputs: %v", err)
	}
	if _, err := subscription.BuildBundle(inputs, generatedAt); err != nil {
		t.Fatalf("build subscription bundle: %v", err)
	}
}

// TestCloneAllocatesAPIPort 验证克隆启用 stats API 的实例时会重分配 API 端口。
func TestCloneAllocatesAPIPort(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	edge, err := Add(baseDir, AddOptions{Name: "edge-api", Template: "edge", AllocatePorts: true})
	if err != nil {
		t.Fatalf("add edge: %v", err)
	}
	path := filepath.Join(baseDir, "instances", "edge-api.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read instance: %v", err)
	}
	updated := strings.Replace(string(data), "enabled: false", "enabled: true", 1)
	if err := os.WriteFile(path, []byte(updated), 0640); err != nil {
		t.Fatalf("enable api: %v", err)
	}

	cloned, err := Clone(baseDir, CloneOptions{Source: "edge-api", Target: "edge-api-copy", AllocatePorts: true})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if cloned.API.Listen == edge.API.Listen {
		t.Fatalf("clone API listen should be reallocated, got %s", cloned.API.Listen)
	}
	if !strings.HasSuffix(cloned.API.Listen, ":10000") {
		t.Fatalf("clone API listen = %s, want first API range port 10000", cloned.API.Listen)
	}
	if _, err := config.LoadAgentConfigSet(baseDir); err != nil {
		t.Fatalf("load config set after clone: %v", err)
	}
}

// TestTemplateSubscriptionRemarksAreUnique 验证多个默认模板可直接导出订阅 bundle。
func TestTemplateSubscriptionRemarksAreUnique(t *testing.T) {
	baseDir := t.TempDir()
	if err := Init(baseDir, InitOptions{ExternalHost: "proxy.example.com"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "edge-smoke", Template: "edge", AllocatePorts: true}); err != nil {
		t.Fatalf("add edge: %v", err)
	}
	if _, err := Add(baseDir, AddOptions{Name: "urltest-smoke", Template: "urltest", Members: []string{"edge-smoke"}, AllocatePorts: true}); err != nil {
		t.Fatalf("add urltest: %v", err)
	}
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		t.Fatalf("load config set: %v", err)
	}
	generatedAt := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	inputs, err := singbox.BuildSubscriptionInputs(set.Global, set.Instances, generatedAt)
	if err != nil {
		t.Fatalf("build subscription inputs: %v", err)
	}
	if _, err := subscription.BuildBundle(inputs, generatedAt); err != nil {
		t.Fatalf("build subscription bundle: %v", err)
	}
}

// hasRefOutbound 判断 instance 是否包含指定 ref outbound。
func hasRefOutbound(instance domain.Instance, name string, ref string) bool {
	for _, outbound := range instance.Outbounds {
		if outbound.Name == name && outbound.Type == "ref" && outbound.Ref == ref {
			return true
		}
	}
	return false
}

// groupHasOutbound 判断 group 是否包含指定 outbound。
func groupHasOutbound(groups []domain.Group, groupName string, outboundName string) bool {
	for _, group := range groups {
		if group.Name != groupName {
			continue
		}
		for _, current := range group.Outbounds {
			if current == outboundName {
				return true
			}
		}
	}
	return false
}

// findTestInbound 按名称查找测试用 inbound。
func findTestInbound(inbounds []domain.Inbound, name string) *domain.Inbound {
	for index := range inbounds {
		if inbounds[index].Name == name {
			return &inbounds[index]
		}
	}
	return nil
}

// TestEditFileWithCommandFindsDefaultEditor 验证未指定 editor 时按预设顺序查找系统编辑器。
func TestEditFileWithCommandFindsDefaultEditor(t *testing.T) {
	tests := []struct {
		name      string
		commands  []string
		wantValue string
	}{
		{name: "uses vim first", commands: []string{"vim", "vi", "nvim", "nano"}, wantValue: "vim"},
		{name: "falls back to vi", commands: []string{"vi", "nvim", "nano"}, wantValue: "vi"},
		{name: "falls back to nvim", commands: []string{"nvim", "nano"}, wantValue: "nvim"},
		{name: "falls back to nano", commands: []string{"nano"}, wantValue: "nano"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			for _, command := range tt.commands {
				writeEditorCommand(t, binDir, command)
			}
			t.Setenv("EDITOR", "")
			t.Setenv("PATH", binDir)

			target := filepath.Join(t.TempDir(), "draft.json")
			if err := os.WriteFile(target, []byte("original"), 0640); err != nil {
				t.Fatalf("write draft: %v", err)
			}
			if err := EditFileWithCommand(target, ""); err != nil {
				t.Fatalf("edit file: %v", err)
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("read draft: %v", err)
			}
			if string(data) != tt.wantValue {
				t.Fatalf("draft = %q, want %q", string(data), tt.wantValue)
			}
		})
	}
}

// writeEditorCommand 写入测试用编辑器命令，执行时把命令名写回目标文件。
func writeEditorCommand(t *testing.T, binDir string, name string) {
	t.Helper()

	path := filepath.Join(binDir, name)
	script := "#!/bin/sh\nprintf '" + name + "' > \"$1\"\n"
	if err := os.WriteFile(path, []byte(script), 0750); err != nil {
		t.Fatalf("write editor command: %v", err)
	}
}
