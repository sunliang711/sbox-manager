package domain

import (
	"errors"
	"strings"
	"testing"
)

// TestDefaultConfigsValidate 验证默认配置能够通过校验。
func TestDefaultConfigsValidate(t *testing.T) {
	if err := ValidateGlobalConfig(DefaultGlobalConfig()); err != nil {
		t.Fatalf("default global config should validate: %v", err)
	}
	if err := ValidateSubConfig(DefaultSubConfig()); err != nil {
		t.Fatalf("default sub config should validate: %v", err)
	}
	if err := ValidateTrafficConfig(DefaultTrafficConfig()); err != nil {
		t.Fatalf("default traffic config should validate: %v", err)
	}
}

// TestPublicSocksHTTPNoauthFails 验证public socks/http 默认禁止 noauth。
func TestPublicSocksHTTPNoauthFails(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds[0] = Inbound{
		Name:   "public-socks",
		Type:   "socks5",
		Listen: "0.0.0.0",
		Port:   24000,
		Auth: AuthConfig{
			Type: "noauth",
		},
	}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected public noauth error")
	}
	if !strings.Contains(err.Error(), "public socks/http") {
		t.Fatalf("expected public noauth error, got %v", err)
	}
}

// TestAPIPublicListenRequiresToken 验证 API 非 loopback 监听必须配置 token。
func TestAPIPublicListenRequiresToken(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.API = APIConfig{
		Enabled: true,
		Listen:  "0.0.0.0:10085",
	}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected public API token error")
	}
	if !strings.Contains(err.Error(), "api.token") {
		t.Fatalf("expected api.token error, got %v", err)
	}
}

// TestMissingReferencesAreAggregated 验证引用不存在时返回聚合错误。
func TestMissingReferencesAreAggregated(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Groups = []Group{
		{
			Name:      "auto",
			Type:      "selector",
			Outbounds: []string{"missing-a"},
		},
	}
	instance.Route = RouteConfig{
		Default: "missing-b",
		Rules: []RouteRule{
			{
				Type:     "domain_suffix",
				Values:   []string{"example.com"},
				Outbound: "missing-c",
			},
		},
	}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected missing reference errors")
	}
	var validationErr *ValidationErrors
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}
	for _, want := range []string{"missing-a", "missing-b", "missing-c"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected aggregated error to contain %q: %v", want, err)
		}
	}
	if len(validationErr.Issues) < 3 {
		t.Fatalf("expected at least 3 validation issues, got %d", len(validationErr.Issues))
	}
}

// TestPortConflictDetected 验证 enabled instance 的端口冲突会被检测。
func TestPortConflictDetected(t *testing.T) {
	global := DefaultGlobalConfig()
	instances := []Instance{
		validInstance("edge-us", 24000),
		validInstance("edge-sg", 24000),
	}

	err := ValidateConfigSet(global, instances)
	if err == nil {
		t.Fatal("expected port conflict error")
	}
	if !strings.Contains(err.Error(), "port 24000") {
		t.Fatalf("expected port conflict error, got %v", err)
	}
}

// TestUnsupportedTypesFail 验证不支持的类型会导致校验失败。
func TestUnsupportedTypesFail(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds[0].Type = "invalid-inbound"
	instance.Outbounds = []Outbound{{Name: "bad-out", Type: "invalid-outbound"}}
	instance.Groups = []Group{{Name: "bad-group", Type: "invalid-group", Outbounds: []string{"bad-out"}}}
	instance.Route = RouteConfig{
		Default: "bad-out",
		Rules: []RouteRule{
			{Type: "invalid-rule", Values: []string{"example.com"}, Outbound: "bad-out"},
		},
	}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
	for _, want := range []string{"invalid-inbound", "invalid-outbound", "invalid-group", "invalid-rule"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected unsupported type %q in error: %v", want, err)
		}
	}
}

// TestVLESSAnyTLSAndTransportsValidate 验证新增协议和官方 V2Ray transport 类型可通过校验。
func TestVLESSAnyTLSAndTransportsValidate(t *testing.T) {
	for _, transportType := range []string{"http", "ws", "quic", "grpc", "httpupgrade"} {
		global := DefaultGlobalConfig()
		instance := validInstance("edge-"+transportType, 24000)
		instance.Inbounds = []Inbound{
			{
				Name:   "vless-main",
				Type:   "vless",
				Listen: "0.0.0.0",
				Port:   24000,
				Transport: TransportConfig{
					Type: transportType,
				},
				Users: []InboundUser{
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
				Port:   24001,
				TLS: TLSConfig{
					Enabled:         true,
					CertificatePath: "/etc/ssl/sbox/fullchain.pem",
					KeyPath:         "/etc/ssl/sbox/private.key",
				},
				Users: []InboundUser{
					{
						Name:     "alice",
						Password: "change-me",
					},
				},
			},
		}
		instance.Outbounds = []Outbound{
			{
				Name: "direct",
				Type: "direct",
			},
			{
				Name:   "vless-upstream",
				Type:   "vless",
				Server: "vless.example.com",
				Port:   443,
				UUID:   "22222222-2222-4222-8222-222222222222",
				Transport: TransportConfig{
					Type: transportType,
				},
			},
			{
				Name:     "anytls-upstream",
				Type:     "anytls",
				Server:   "anytls.example.com",
				Port:     443,
				Password: "change-me",
				TLS:      TLSConfig{Enabled: true},
			},
		}
		instance.Route = RouteConfig{Default: "direct"}
		ApplyInstanceDefaults(&instance)

		if err := ValidateInstance(global, &instance); err != nil {
			t.Fatalf("transport %s should validate: %v", transportType, err)
		}
	}
}

// TestVLESSFlowRejectsTransport 验证 VLESS flow 不能和 V2Ray transport 混用。
func TestVLESSFlowRejectsTransport(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds = []Inbound{
		{
			Name:   "vless-ws",
			Type:   "vless",
			Listen: "0.0.0.0",
			Port:   24000,
			Transport: TransportConfig{
				Type: "ws",
			},
			Users: []InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
					Flow: "xtls-rprx-vision",
				},
			},
		},
	}
	instance.Outbounds = []Outbound{
		{
			Name:   "vless-ws-upstream",
			Type:   "vless",
			Server: "vless.example.com",
			Port:   443,
			UUID:   "22222222-2222-4222-8222-222222222222",
			Flow:   "xtls-rprx-vision",
			Transport: TransportConfig{
				Type: "ws",
			},
		},
	}
	instance.Route = RouteConfig{Default: "vless-ws-upstream"}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected vless flow with transport error")
	}
	if strings.Count(err.Error(), "cannot be used with transport") < 2 {
		t.Fatalf("expected inbound and outbound flow transport errors, got %v", err)
	}
}

// TestInboundTLSRequiresCertificatePaths 验证启用 TLS 的 inbound 必须配置服务端证书和私钥。
func TestInboundTLSRequiresCertificatePaths(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds[0].TLS = TLSConfig{Enabled: true}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected inbound tls certificate error")
	}
	for _, want := range []string{"certificate_path", "key_path"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %s error, got %v", want, err)
		}
	}
}

// TestVMessWebSocketTLSValidate 验证 VMess WebSocket + TLS 字段可通过校验。
func TestVMessWebSocketTLSValidate(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Outbounds = []Outbound{
		{
			Name:     "vmess-upstream",
			Type:     "vmess",
			Server:   "example.cc",
			Port:     443,
			UUID:     "244a79b1-522f-4f43-8d58-69c88ef732fe",
			Security: "auto",
			TLS: TLSConfig{
				Enabled:    true,
				ServerName: "example.cc",
				Insecure:   false,
				ALPN:       []string{"h2", "http/1.1"},
			},
			Transport: TransportConfig{
				Type: "ws",
				Path: "/vmess-websocket",
				Headers: map[string]string{
					"Host": "example.cc",
				},
			},
		},
	}
	instance.Route = RouteConfig{Default: "vmess-upstream"}

	if err := ValidateInstance(global, &instance); err != nil {
		t.Fatalf("vmess websocket tls should validate: %v", err)
	}
}

// TestInvalidTransportFails 验证未知 V2Ray transport 类型会失败。
func TestInvalidTransportFails(t *testing.T) {
	for _, transportType := range []string{"mkcp", "xhttp"} {
		global := DefaultGlobalConfig()
		instance := validInstance("edge-us", 24000)
		instance.Inbounds[0].Transport = TransportConfig{Type: transportType}

		err := ValidateInstance(global, &instance)
		if err == nil {
			t.Fatalf("expected invalid transport error for %s", transportType)
		}
		if !strings.Contains(err.Error(), transportType) {
			t.Fatalf("expected transport type in error: %v", err)
		}
	}
}

// TestVLESSRealityVisionValidate 验证 VLESS REALITY Vision 组合可通过校验。
func TestVLESSRealityVisionValidate(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds = []Inbound{
		{
			Name:   "vless-reality-vision",
			Type:   "vless",
			Listen: "0.0.0.0",
			Port:   24000,
			TLS: TLSConfig{
				Enabled:    true,
				ServerName: "www.cloudflare.com",
				Reality: RealityConfig{
					Enabled:             true,
					HandshakeServer:     "www.cloudflare.com",
					HandshakeServerPort: 443,
					PrivateKey:          "change-me-reality-private-key",
					PublicKey:           "change-me-reality-public-key",
					ShortIDs:            []string{"0123456789abcdef"},
				},
			},
			Users: []InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
					Flow: "xtls-rprx-vision",
				},
			},
		},
	}
	instance.Outbounds = []Outbound{
		{
			Name:   "vless-reality-vision-upstream",
			Type:   "vless",
			Server: "vless.example.com",
			Port:   443,
			UUID:   "22222222-2222-4222-8222-222222222222",
			Flow:   "xtls-rprx-vision",
			TLS: TLSConfig{
				Enabled:    true,
				ServerName: "www.cloudflare.com",
				Reality: RealityConfig{
					Enabled:   true,
					PublicKey: "change-me-reality-public-key",
					ShortID:   "0123456789abcdef",
				},
				UTLS: UTLSConfig{
					Enabled:     true,
					Fingerprint: "chrome",
				},
			},
		},
	}
	instance.Route = RouteConfig{Default: "vless-reality-vision-upstream"}

	if err := ValidateInstance(global, &instance); err != nil {
		t.Fatalf("vless reality vision should validate: %v", err)
	}
}

// TestVMessNetworkRejectsTransportName 验证 network 不能混用 V2Ray transport 类型。
func TestVMessNetworkRejectsTransportName(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Outbounds = []Outbound{
		{
			Name:    "vmess-upstream",
			Type:    "vmess",
			Server:  "vmess.example.com",
			Port:    443,
			UUID:    "22222222-2222-4222-8222-222222222222",
			Network: "ws",
		},
	}
	instance.Route = RouteConfig{Default: "vmess-upstream"}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected invalid vmess network error")
	}
	if !strings.Contains(err.Error(), "network") {
		t.Fatalf("expected network error, got %v", err)
	}
}

// TestAnyTLSRequiresTLS 验证 AnyTLS inbound/outbound 必须启用 TLS。
func TestAnyTLSRequiresTLS(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds = []Inbound{
		{
			Name:   "anytls-main",
			Type:   "anytls",
			Listen: "0.0.0.0",
			Port:   24000,
			Users: []InboundUser{
				{
					Name:     "alice",
					Password: "change-me",
				},
			},
		},
	}
	instance.Outbounds = []Outbound{
		{
			Name:     "anytls-upstream",
			Type:     "anytls",
			Server:   "anytls.example.com",
			Port:     443,
			Password: "change-me",
		},
	}
	instance.Route = RouteConfig{Default: "anytls-upstream"}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected anytls tls error")
	}
	if !strings.Contains(err.Error(), "tls.enabled") {
		t.Fatalf("expected tls.enabled error, got %v", err)
	}
}

// TestVMessVLESSRejectCrossProtocolUserFields 验证 VMess/VLESS 用户字段不会串协议。
func TestVMessVLESSRejectCrossProtocolUserFields(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds[0].Users[0].Flow = "xtls-rprx-vision"
	instance.Inbounds = append(instance.Inbounds, Inbound{
		Name:   "vless-main",
		Type:   "vless",
		Listen: "0.0.0.0",
		Port:   24001,
		Users: []InboundUser{
			{
				Name:    "alice",
				UUID:    "22222222-2222-4222-8222-222222222222",
				AlterID: 1,
			},
		},
	})

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected cross protocol field errors")
	}
	for _, want := range []string{"flow", "alter_id"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected %s error, got %v", want, err)
		}
	}
}

// TestOutboundRefRequiresExistingSocksOrHTTPInbound 验证 ref outbound 必须指向已有 socks5/http inbound。
func TestOutboundRefRequiresExistingSocksOrHTTPInbound(t *testing.T) {
	global := DefaultGlobalConfig()
	member := validInstance("edge.us", 24000)
	member.Inbounds = append(member.Inbounds, Inbound{
		Name:   "local-socks",
		Type:   "socks5",
		Listen: "127.0.0.1",
		Port:   17000,
		Auth:   AuthConfig{Type: "noauth"},
	})
	auto := validInstance("auto-us", 24001)
	auto.Outbounds = []Outbound{
		{
			Name: "edge.us-local-socks",
			Type: "ref",
			Ref:  "edge.us.local-socks",
		},
	}
	auto.Groups = []Group{
		{
			Name:      "auto",
			Type:      "urltest",
			Outbounds: []string{"edge.us-local-socks"},
			URL:       "http://www.gstatic.com/generate_204",
			Interval:  300,
		},
	}
	auto.Route = RouteConfig{Default: "auto"}

	if err := ValidateConfigSet(global, []Instance{member, auto}); err != nil {
		t.Fatalf("ref outbound should validate: %v", err)
	}
}

// TestOutboundRefRejectsMissingTarget 验证 ref outbound 指向不存在的目标会失败。
func TestOutboundRefRejectsMissingTarget(t *testing.T) {
	global := DefaultGlobalConfig()
	auto := validInstance("auto-us", 24000)
	auto.Outbounds = []Outbound{
		{
			Name: "missing-local-socks",
			Type: "ref",
			Ref:  "missing.local-socks",
		},
	}
	auto.Route = RouteConfig{Default: "missing-local-socks"}

	err := ValidateConfigSet(global, []Instance{auto})
	if err == nil {
		t.Fatal("expected missing ref target error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing ref target in error: %v", err)
	}
}

// validInstance 返回可通过校验的 instance fixture。
func validInstance(name string, port int) Instance {
	instance := DefaultInstance(DefaultGlobalConfig())
	instance.Name = name
	instance.API.Enabled = false
	instance.Inbounds = []Inbound{
		{
			Name:   "vmess-main",
			Type:   "vmess",
			Listen: "0.0.0.0",
			Port:   port,
			Users: []InboundUser{
				{
					Name: "alice",
					UUID: "11111111-1111-4111-8111-111111111111",
				},
			},
		},
	}
	instance.Outbounds = []Outbound{
		{
			Name: "direct",
			Type: "direct",
		},
	}
	instance.Route = RouteConfig{
		Default: "direct",
	}
	ApplyInstanceDefaults(&instance)
	return instance
}
