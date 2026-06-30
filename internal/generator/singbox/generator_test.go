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
			UDP:    true,
			TLS: domain.TLSConfig{
				Enabled:         true,
				CertificatePath: "/etc/ssl/sbox/fullchain.pem",
				KeyPath:         "/etc/ssl/sbox/private.key",
			},
			Transport: domain.TransportConfig{
				Type:        "grpc",
				ServiceName: "TunService",
			},
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
				},
			},
		},
		{
			Name:   "anytls-main",
			Type:   "anytls",
			Listen: "0.0.0.0",
			Port:   24102,
			TLS: domain.TLSConfig{
				Enabled:         true,
				CertificatePath: "/etc/ssl/sbox/fullchain.pem",
				KeyPath:         "/etc/ssl/sbox/private.key",
			},
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
				Type:   "httpupgrade",
				Host:   "vless.example.com",
				Path:   "/upgrade",
				Method: "GET",
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
	for _, want := range []string{`"type": "vless"`, `"type": "anytls"`, `"type": "grpc"`, `"service_name": "TunService"`, `"type": "httpupgrade"`, `"path": "/upgrade"`, `"certificate_path": "/etc/ssl/sbox/fullchain.pem"`, `"key_path": "/etc/ssl/sbox/private.key"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated config missing %s: %s", want, output)
		}
	}
	if strings.Contains(output, `"method":`) {
		t.Fatalf("generated httpupgrade transport should not emit method field: %s", output)
	}
	if strings.Contains(output, `"udp"`) {
		t.Fatalf("generated config should not emit unsupported inbound udp field: %s", output)
	}
}

// TestGenerateVLESSRealityVision 验证 VLESS REALITY Vision 字段生成到 sing-box JSON。
func TestGenerateVLESSRealityVision(t *testing.T) {
	global, instance := testConfig()
	instance.Inbounds = []domain.Inbound{
		{
			Name:   "vless-reality-vision",
			Type:   "vless",
			Listen: "0.0.0.0",
			Port:   24110,
			TLS: domain.TLSConfig{
				Enabled:    true,
				ServerName: "www.cloudflare.com",
				Reality: domain.RealityConfig{
					Enabled:             true,
					HandshakeServer:     "www.cloudflare.com",
					HandshakeServerPort: 443,
					PrivateKey:          "change-me-reality-private-key",
					PublicKey:           "change-me-reality-public-key",
					ShortIDs:            []string{"0123456789abcdef"},
				},
			},
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
					Flow: "xtls-rprx-vision",
				},
			},
		},
	}
	instance.Outbounds = []domain.Outbound{
		{
			Name:   "vless-reality-vision-upstream",
			Type:   "vless",
			Server: "vless.example.com",
			Port:   443,
			UUID:   "22222222-2222-4222-8222-222222222222",
			Flow:   "xtls-rprx-vision",
			TLS: domain.TLSConfig{
				Enabled:    true,
				ServerName: "www.cloudflare.com",
				Reality: domain.RealityConfig{
					Enabled:   true,
					PublicKey: "change-me-reality-public-key",
					ShortID:   "0123456789abcdef",
				},
				UTLS: domain.UTLSConfig{
					Enabled:     true,
					Fingerprint: "chrome",
				},
			},
		},
	}
	instance.Groups = nil
	instance.Route = domain.RouteConfig{Default: "vless-reality-vision-upstream"}
	domain.ApplyInstanceDefaults(&instance)

	generated, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	output := string(generated)
	for _, want := range []string{`"flow": "xtls-rprx-vision"`, `"reality": {`, `"handshake": {`, `"server": "www.cloudflare.com"`, `"server_port": 443`, `"private_key": "change-me-reality-private-key"`, `"short_id": [`, `"public_key": "change-me-reality-public-key"`, `"utls": {`, `"fingerprint": "chrome"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated reality config missing %s: %s", want, output)
		}
	}
	if strings.Contains(output, `"certificate_path":`) || strings.Contains(output, `"key_path":`) {
		t.Fatalf("generated reality config should not require certificate files: %s", output)
	}
}

// TestGenerateVMessWebSocketTLS 验证 VMess WebSocket + TLS 字段生成到 sing-box JSON。
func TestGenerateVMessWebSocketTLS(t *testing.T) {
	global, instance := testConfig()
	instance.Outbounds = []domain.Outbound{
		{
			Name:     "vmess-upstream",
			Type:     "vmess",
			Server:   "example.cc",
			Port:     443,
			UUID:     "244a79b1-522f-4f43-8d58-69c88ef732fe",
			Security: "auto",
			TLS: domain.TLSConfig{
				Enabled:    true,
				ServerName: "example.cc",
				Insecure:   true,
				ALPN:       []string{"h2", "http/1.1"},
			},
			Transport: domain.TransportConfig{
				Type: "ws",
				Path: "/vmess-websocket",
				Headers: map[string]string{
					"Host": "example.cc",
				},
			},
		},
	}
	instance.Groups = nil
	instance.Route = domain.RouteConfig{Default: "vmess-upstream"}
	domain.ApplyInstanceDefaults(&instance)

	generated, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	output := string(generated)
	for _, want := range []string{`"type": "vmess"`, `"security": "auto"`, `"tls": {`, `"server_name": "example.cc"`, `"insecure": true`, `"alpn": [`, `"type": "ws"`, `"path": "/vmess-websocket"`, `"Host": "example.cc"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated config missing %s: %s", want, output)
		}
	}
	if strings.Contains(output, `"host":`) {
		t.Fatalf("generated websocket transport should not emit host field: %s", output)
	}
}

// TestGenerateShadowsocksInboundUsesTopLevelMethod 验证 Shadowsocks inbound 只在顶层生成 method。
func TestGenerateShadowsocksInboundUsesTopLevelMethod(t *testing.T) {
	global, instance := testConfig()
	instance.Inbounds = []domain.Inbound{
		{
			Name:   "ss-main",
			Type:   "shadowsocks",
			Listen: "127.0.0.1",
			Port:   24200,
			Method: "2022-blake3-aes-256-gcm",
			Users: []domain.InboundUser{
				{
					Name:     "alice",
					Password: "change-me-32-byte-key",
					Method:   "2022-blake3-aes-256-gcm",
				},
			},
		},
	}
	instance.Outbounds = []domain.Outbound{{Name: "direct", Type: "direct"}}
	instance.Groups = nil
	instance.Route = domain.RouteConfig{Default: "direct"}
	domain.ApplyInstanceDefaults(&instance)

	generated, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	output := string(generated)
	if strings.Count(output, `"method"`) != 1 {
		t.Fatalf("generated config should emit only top-level shadowsocks method: %s", output)
	}
}

// TestGenerateInboundWebSocketOmitsClientHost 验证 inbound WebSocket 不生成客户端侧 host 字段。
func TestGenerateInboundWebSocketOmitsClientHost(t *testing.T) {
	global, instance := testConfig()
	instance.Inbounds = []domain.Inbound{
		{
			Name:   "vmess-ws",
			Type:   "vmess",
			Listen: "127.0.0.1",
			Port:   24101,
			Transport: domain.TransportConfig{
				Type: "ws",
				Host: "proxy.example.com",
				Path: "/vmess-websocket",
				Headers: map[string]string{
					"Host": "proxy.example.com",
				},
			},
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
				},
			},
		},
	}
	instance.Outbounds = []domain.Outbound{{Name: "direct", Type: "direct"}}
	instance.Groups = nil
	instance.Route = domain.RouteConfig{Default: "direct"}
	domain.ApplyInstanceDefaults(&instance)

	generated, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	output := string(generated)
	if strings.Contains(output, `"host":`) {
		t.Fatalf("generated inbound transport should not emit host field: %s", output)
	}
	if !strings.Contains(output, `"Host": "proxy.example.com"`) {
		t.Fatalf("generated inbound transport should keep headers: %s", output)
	}
}

// TestGenerateInboundHTTPUpgradeOmitsClientMethod 验证 inbound HTTPUpgrade 不生成客户端侧 method 字段。
func TestGenerateInboundHTTPUpgradeOmitsClientMethod(t *testing.T) {
	global, instance := testConfig()
	instance.Inbounds = []domain.Inbound{
		{
			Name:   "vmess-upgrade",
			Type:   "vmess",
			Listen: "127.0.0.1",
			Port:   24103,
			Transport: domain.TransportConfig{
				Type:   "httpupgrade",
				Host:   "proxy.example.com",
				Path:   "/upgrade",
				Method: "GET",
				Headers: map[string]string{
					"Host": "proxy.example.com",
				},
			},
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
				},
			},
		},
	}
	instance.Outbounds = []domain.Outbound{{Name: "direct", Type: "direct"}}
	instance.Groups = nil
	instance.Route = domain.RouteConfig{Default: "direct"}
	domain.ApplyInstanceDefaults(&instance)

	generated, err := Generate(global, instance)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	output := string(generated)
	if strings.Contains(output, `"method":`) {
		t.Fatalf("generated inbound httpupgrade transport should not emit method field: %s", output)
	}
	if !strings.Contains(output, `"type": "httpupgrade"`) || !strings.Contains(output, `"path": "/upgrade"`) {
		t.Fatalf("generated inbound httpupgrade transport missing expected fields: %s", output)
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
			TLS: domain.TLSConfig{
				Enabled:    true,
				ServerName: "www.cloudflare.com",
				Reality: domain.RealityConfig{
					Enabled:             true,
					HandshakeServer:     "www.cloudflare.com",
					HandshakeServerPort: 443,
					PrivateKey:          "change-me-reality-private-key",
					PublicKey:           "change-me-reality-public-key",
					ShortIDs:            []string{"0123456789abcdef"},
				},
			},
			Transport: domain.TransportConfig{
				Type: "ws",
				Path: "/ws",
			},
			Users: []domain.InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
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
			TLS: domain.TLSConfig{
				Enabled:         true,
				CertificatePath: "/etc/ssl/sbox/fullchain.pem",
				KeyPath:         "/etc/ssl/sbox/private.key",
			},
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
	if input.Nodes[0].Protocol != "vless" || input.Nodes[0].Transport.Type != "ws" || input.Nodes[0].Flow != "" {
		t.Fatalf("vless node missing fields: %+v", input.Nodes[0])
	}
	if !input.Nodes[0].TLS.Reality.Enabled || input.Nodes[0].TLS.Reality.PrivateKey != "" || input.Nodes[0].TLS.Reality.PublicKey != "change-me-reality-public-key" {
		t.Fatalf("vless node should keep public reality fields only: %+v", input.Nodes[0].TLS.Reality)
	}
	if !input.Nodes[0].TLS.UTLS.Enabled || input.Nodes[0].TLS.UTLS.Fingerprint != "chrome" {
		t.Fatalf("vless reality node should include client utls fingerprint: %+v", input.Nodes[0].TLS.UTLS)
	}
	if input.Nodes[1].Protocol != "anytls" || input.Nodes[1].Password != "change-me" || !input.Nodes[1].TLS.Enabled {
		t.Fatalf("anytls node missing fields: %+v", input.Nodes[1])
	}
	if input.Nodes[1].TLS.CertificatePath != "" || input.Nodes[1].TLS.KeyPath != "" {
		t.Fatalf("subscription node should not expose server certificate paths: %+v", input.Nodes[1].TLS)
	}
}

// TestGenerateWithInstancesResolvesRefOutbound 验证 ref outbound 会在生成阶段解析为 socks/http outbound。
func TestGenerateWithInstancesResolvesRefOutbound(t *testing.T) {
	global, auto := testConfig()
	member := domain.DefaultInstance(global)
	member.Name = "edge.us"
	member.API.Enabled = false
	member.Inbounds = []domain.Inbound{
		{
			Name:   "local-socks",
			Type:   "socks5",
			Listen: "0.0.0.0",
			Port:   17000,
			Auth: domain.AuthConfig{
				Type:     "password",
				Username: "alice",
				Password: "change-me",
			},
		},
	}
	member.Outbounds = []domain.Outbound{{Name: "direct", Type: "direct"}}
	member.Route = domain.RouteConfig{Default: "direct"}
	domain.ApplyInstanceDefaults(&member)

	auto.Name = "auto-us"
	auto.Outbounds = []domain.Outbound{
		{
			Name: "edge.us-local-socks",
			Type: "ref",
			Ref:  "edge.us.local-socks",
		},
	}
	auto.Groups = []domain.Group{
		{
			Name:      "auto",
			Type:      "urltest",
			Outbounds: []string{"edge.us-local-socks"},
			URL:       "http://www.gstatic.com/generate_204",
			Interval:  300,
		},
	}
	auto.Route = domain.RouteConfig{Default: "auto"}

	generated, err := GenerateWithInstances(global, []domain.Instance{member, auto}, auto)
	if err != nil {
		t.Fatalf("generate ref config: %v", err)
	}
	output := string(generated)
	for _, want := range []string{`"type": "socks"`, `"tag": "edge.us-local-socks"`, `"server": "127.0.0.1"`, `"server_port": 17000`, `"username": "alice"`, `"password": "change-me"`} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated ref config missing %s: %s", want, output)
		}
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
