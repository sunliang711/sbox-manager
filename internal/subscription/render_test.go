package subscription

import (
	"strings"
	"testing"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestSurgeRendersVMessWebSocketAEADAndSkipsVLESS 验证 Surge 支持 VMess WebSocket/AEAD 并跳过 VLESS。
func TestSurgeRendersVMessWebSocketAEADAndSkipsVLESS(t *testing.T) {
	nodes := []domain.SubscriptionNode{
		{
			Protocol: "vmess",
			Server:   "vmess.example.com",
			Port:     443,
			Remark:   "US VMess",
			UUID:     "11111111-1111-4111-8111-111111111111",
			Network:  "tcp",
			Transport: domain.TransportConfig{
				Type: "ws",
				Path: "/ws",
				Headers: map[string]string{
					"X-Test": "yes",
					"Host":   "vmess.example.com",
				},
			},
		},
		{
			Protocol: "vless",
			Server:   "vless.example.com",
			Port:     443,
			Remark:   "US VLESS",
			UUID:     "22222222-2222-4222-8222-222222222222",
		},
		{
			Protocol: "anytls",
			Server:   "anytls.example.com",
			Port:     443,
			Remark:   "US AnyTLS",
			Password: "change-me",
			TLS:      domain.TLSConfig{Enabled: true},
		},
	}

	filtered := FilterNodesForFormat(FormatSurge, nodes)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 surge nodes, got %d: %+v", len(filtered), filtered)
	}
	text := strings.Join(surgeProxyLines(filtered), "\n")
	for _, want := range []string{"US VMess = vmess", "ws=true", "ws-path=/ws", "ws-headers=Host:vmess.example.com|X-Test:yes", "vmess-aead=true", "US AnyTLS = anytls"} {
		if !strings.Contains(text, want) {
			t.Fatalf("surge output missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "VLESS") {
		t.Fatalf("surge output should skip vless: %s", text)
	}
}

// TestSingBoxSubscriptionRendersVLESSTransport 验证 sing-box 订阅保留 VLESS transport。
func TestSingBoxSubscriptionRendersVLESSTransport(t *testing.T) {
	nodes := []domain.SubscriptionNode{
		{
			Protocol: "vless",
			Server:   "vless.example.com",
			Port:     443,
			Tag:      "vless-main",
			Remark:   "US VLESS",
			UUID:     "22222222-2222-4222-8222-222222222222",
			TLS:      domain.TLSConfig{Enabled: true},
			Transport: domain.TransportConfig{
				Type:   "httpupgrade",
				Host:   "vless.example.com",
				Path:   "/upgrade",
				Method: "GET",
			},
		},
	}

	outboundsJSON, err := singBoxOutboundsJSON(nodes)
	if err != nil {
		t.Fatalf("render sing-box outbounds: %v", err)
	}
	for _, want := range []string{`"type": "vless"`, `"tls": {`, `"type": "httpupgrade"`, `"host": "vless.example.com"`, `"path": "/upgrade"`} {
		if !strings.Contains(outboundsJSON, want) {
			t.Fatalf("sing-box output missing %s: %s", want, outboundsJSON)
		}
	}
	if strings.Contains(outboundsJSON, `"network": "httpupgrade"`) {
		t.Fatalf("sing-box output should not mix transport into network: %s", outboundsJSON)
	}
	if strings.Contains(outboundsJSON, `"method":`) {
		t.Fatalf("sing-box output should not emit unsupported httpupgrade method: %s", outboundsJSON)
	}
}

// TestSingBoxSubscriptionSkipsUnsupportedOutboundFields 验证 sing-box 订阅不会输出当前 sing-box outbound 不支持的字段。
func TestSingBoxSubscriptionSkipsUnsupportedOutboundFields(t *testing.T) {
	nodes := []domain.SubscriptionNode{
		{
			Protocol: "vmess",
			Server:   "vmess.example.com",
			Port:     443,
			Tag:      "vmess-ws",
			Remark:   "US VMess WS",
			UUID:     "11111111-1111-4111-8111-111111111111",
			Network:  "tcp",
			UDP:      true,
			Transport: domain.TransportConfig{
				Type: "ws",
				Host: "vmess.example.com",
				Path: "/ws",
				Headers: map[string]string{
					"Host": "vmess.example.com",
				},
			},
		},
	}

	outboundsJSON, err := singBoxOutboundsJSON(nodes)
	if err != nil {
		t.Fatalf("render sing-box outbounds: %v", err)
	}
	for _, want := range []string{`"type": "vmess"`, `"network": "tcp"`, `"type": "ws"`, `"headers": {`, `"Host": "vmess.example.com"`} {
		if !strings.Contains(outboundsJSON, want) {
			t.Fatalf("sing-box output missing %s: %s", want, outboundsJSON)
		}
	}
	for _, unwanted := range []string{`"udp":`, `"host": "vmess.example.com"`} {
		if strings.Contains(outboundsJSON, unwanted) {
			t.Fatalf("sing-box output should not contain %s: %s", unwanted, outboundsJSON)
		}
	}
}

// TestSingBoxSubscriptionRendersVLESSRealityVision 验证 sing-box 订阅保留 VLESS REALITY Vision 字段。
func TestSingBoxSubscriptionRendersVLESSRealityVision(t *testing.T) {
	nodes := []domain.SubscriptionNode{
		{
			Protocol: "vless",
			Server:   "vless.example.com",
			Port:     443,
			Tag:      "vless-reality-vision",
			Remark:   "US VLESS REALITY Vision",
			UUID:     "22222222-2222-4222-8222-222222222222",
			Flow:     "xtls-rprx-vision",
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

	outboundsJSON, err := singBoxOutboundsJSON(nodes)
	if err != nil {
		t.Fatalf("render sing-box outbounds: %v", err)
	}
	for _, want := range []string{`"flow": "xtls-rprx-vision"`, `"server_name": "www.cloudflare.com"`, `"reality": {`, `"public_key": "change-me-reality-public-key"`, `"short_id": "0123456789abcdef"`, `"utls": {`, `"fingerprint": "chrome"`} {
		if !strings.Contains(outboundsJSON, want) {
			t.Fatalf("sing-box output missing %s: %s", want, outboundsJSON)
		}
	}
	if strings.Contains(outboundsJSON, `"private_key":`) {
		t.Fatalf("sing-box output should not expose reality private key: %s", outboundsJSON)
	}
}
