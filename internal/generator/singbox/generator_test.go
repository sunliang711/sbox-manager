package singbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestGenerateStableBytes 验证相同输入生成字节级一致的 JSON。
func TestGenerateStableBytes(t *testing.T) {
	global, instance := testConfig()

	first, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate first config: %v", err)
	}
	second, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate second config: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("expected stable bytes\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	output := string(first)
	for _, want := range []string{`"log"`, `"dns"`, `"inbounds"`, `"outbounds"`, `"route"`, `"experimental"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated config missing %s: %s", want, output)
		}
	}
	if strings.Contains(output, `"address": "local"`) {
		t.Fatalf("generated config should use new DNS server format: %s", output)
	}
	if !strings.Contains(output, `"type": "local"`) {
		t.Fatalf("generated config missing new local DNS server type: %s", output)
	}
	assertOrder(t, output, `"log"`, `"dns"`)
	assertOrder(t, output, `"dns"`, `"inbounds"`)
	assertOrder(t, output, `"inbounds"`, `"outbounds"`)
	assertOrder(t, output, `"outbounds"`, `"route"`)
	assertOrder(t, output, `"route"`, `"experimental"`)
}

// TestGenerateMatchesGolden 验证核心 edge 实例输出与 golden 文件保持一致。
func TestGenerateMatchesGolden(t *testing.T) {
	global, instance := testConfig()
	generated, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "edge-us.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if string(generated) != string(golden) {
		t.Fatalf("generated output differs from golden\nwant:\n%s\ngot:\n%s", golden, generated)
	}
}

// TestBuildSubscriptionInputsRequiresExternalHost 验证订阅渲染缺少 external_host 时失败。
func TestBuildSubscriptionInputsRequiresExternalHost(t *testing.T) {
	global, instance := testConfig()
	global.ExternalHost = ""

	_, err := BuildSubscriptionInput(global, instance, testTime())
	if err == nil {
		t.Fatal("expected external_host error")
	}
	if !strings.Contains(err.Error(), "external_host") {
		t.Fatalf("expected external_host error, got %v", err)
	}
}

// TestGenerateSupportsVLESSAnyTLSAndTransports 验证新增协议和 transport 能生成 sing-box JSON。
func TestGenerateSupportsVLESSAnyTLSAndTransports(t *testing.T) {
	global, instance := testConfig()
	instance.Inbounds = []domain.Inbound{
		{
			Name:   "vless-main",
			Type:   "vless",
			Listen: "0.0.0.0",
			Port:   24101,
			TLS:    domain.TLSConfig{Enabled: true},
			Transport: domain.TransportConfig{
				Type:        "grpc",
				ServiceName: "TunService",
			},
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
					Flow: "xtls-rprx-vision",
				},
			},
		},
		{
			Name:   "anytls-main",
			Type:   "anytls",
			Listen: "0.0.0.0",
			Port:   24102,
			TLS:    domain.TLSConfig{Enabled: true},
			Users: []domain.InboundUser{
				{
					Name:     "alice",
					Password: "change-me",
				},
			},
		},
	}
	instance.Outbounds = []domain.Outbound{
		{
			Name:   "vless-upstream",
			Type:   "vless",
			Server: "vless.example.com",
			Port:   443,
			UUID:   "22222222-2222-4222-8222-222222222222",
			Transport: domain.TransportConfig{
				Type: "httpupgrade",
				Host: "vless.example.com",
				Path: "/upgrade",
			},
		},
		{
			Name:     "anytls-upstream",
			Type:     "anytls",
			Server:   "anytls.example.com",
			Port:     443,
			Password: "change-me",
			TLS:      domain.TLSConfig{Enabled: true},
		},
	}
	instance.Groups = nil
	instance.Route = domain.RouteConfig{Default: "vless-upstream"}
	domain.ApplyInstanceDefaults(&instance)

	generated, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	output := string(generated)
	for _, want := range []string{`"type": "vless"`, `"type": "anytls"`, `"type": "grpc"`, `"service_name": "TunService"`, `"type": "httpupgrade"`, `"path": "/upgrade"`, `"flow": "xtls-rprx-vision"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated config missing %s: %s", want, output)
		}
	}
}

// TestBuildSubscriptionInputIncludesNewProtocolFields 验证 inbound 订阅节点保留新增协议字段。
func TestBuildSubscriptionInputIncludesNewProtocolFields(t *testing.T) {
	global, instance := testConfig()
	instance.Inbounds = []domain.Inbound{
		{
			Name:   "vless-main",
			Type:   "vless",
			Listen: "0.0.0.0",
			Port:   24101,
			Transport: domain.TransportConfig{
				Type: "ws",
				Path: "/ws",
			},
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
					Flow: "xtls-rprx-vision",
				},
			},
			Subscription: domain.SubscriptionConfig{
				Enabled: true,
				User:    "alice",
				Remark:  "US VLESS",
				Region:  "US",
			},
		},
		{
			Name:   "anytls-main",
			Type:   "anytls",
			Listen: "0.0.0.0",
			Port:   24102,
			TLS:    domain.TLSConfig{Enabled: true},
			Users: []domain.InboundUser{
				{
					Name:     "alice",
					Password: "change-me",
				},
			},
			Subscription: domain.SubscriptionConfig{
				Enabled: true,
				User:    "alice",
				Remark:  "US AnyTLS",
				Region:  "US",
			},
		},
	}
	domain.ApplyInstanceDefaults(&instance)

	input, err := BuildSubscriptionInput(global, instance, testTime())
	if err != nil {
		t.Fatalf("build subscription input: %v", err)
	}
	if len(input.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(input.Nodes))
	}
	if input.Nodes[0].Protocol != "vless" || input.Nodes[0].Transport.Type != "ws" || input.Nodes[0].Flow != "xtls-rprx-vision" {
		t.Fatalf("vless node missing fields: %+v", input.Nodes[0])
	}
	if input.Nodes[1].Protocol != "anytls" || input.Nodes[1].Password != "change-me" || !input.Nodes[1].TLS.Enabled {
		t.Fatalf("anytls node missing fields: %+v", input.Nodes[1])
	}
}

// testConfig 返回生成器测试使用的稳定配置。
func testConfig() (domain.GlobalConfig, domain.Instance) {
	global := domain.DefaultGlobalConfig()
	global.ExternalHost = "proxy.example.com"
	instance := domain.DefaultInstance(global)
	instance.Name = "edge-us"
	instance.API.Enabled = true
	instance.API.Listen = "127.0.0.1:10085"
	instance.Inbounds = []domain.Inbound{
		{
			Name:   "vmess-main",
			Type:   "vmess",
			Listen: "0.0.0.0",
			Port:   24100,
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
				},
			},
			Subscription: domain.SubscriptionConfig{
				Enabled: true,
				User:    "alice",
				Remark:  "US VMess",
				Region:  "US",
			},
		},
	}
	instance.Outbounds = []domain.Outbound{
		{
			Name: "direct",
			Type: "direct",
		},
		{
			Name:     "proxy-a",
			Type:     "shadowsocks",
			Server:   "server.example.com",
			Port:     443,
			Method:   "2022-blake3-aes-256-gcm",
			Password: "change-me",
		},
	}
	instance.Groups = []domain.Group{
		{
			Name:      "auto",
			Type:      "urltest",
			Outbounds: []string{"proxy-a", "direct"},
			URL:       "http://www.gstatic.com/generate_204",
			Interval:  300,
		},
	}
	instance.Route = domain.RouteConfig{
		Default: "auto",
		Rules: []domain.RouteRule{
			{
				Type:     "domain_suffix",
				Values:   []string{"google.com"},
				Outbound: "auto",
			},
		},
	}
	domain.ApplyInstanceDefaults(&instance)
	return global, instance
}

// testTime 返回订阅测试使用的固定时间。
func testTime() time.Time {
	return time.Date(2026, 6, 28, 12, 0, 0, 0, time.FixedZone("CST", 8*60*60))
}

// assertOrder 验证左侧片段在右侧片段之前出现。
func assertOrder(t *testing.T, text string, left string, right string) {
	t.Helper()
	leftIndex := strings.Index(text, left)
	rightIndex := strings.Index(text, right)
	if leftIndex < 0 || rightIndex < 0 || leftIndex >= rightIndex {
		t.Fatalf("expected %s before %s in %s", left, right, text)
	}
}
