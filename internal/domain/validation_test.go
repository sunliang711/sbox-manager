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

// TestPublicSocksHTTPNoauthFails 验证公开 socks/http 默认禁止 noauth。
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
	if !strings.Contains(err.Error(), "公开 socks/http") {
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
	if !strings.Contains(err.Error(), "端口 24000") {
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
						Flow: "xtls-rprx-vision",
					},
				},
			},
			{
				Name:   "anytls-main",
				Type:   "anytls",
				Listen: "0.0.0.0",
				Port:   24001,
				TLS:    TLSConfig{Enabled: true},
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

// TestInvalidTransportFails 验证未知 V2Ray transport 类型会失败。
func TestInvalidTransportFails(t *testing.T) {
	global := DefaultGlobalConfig()
	instance := validInstance("edge-us", 24000)
	instance.Inbounds[0].Transport = TransportConfig{Type: "mkcp"}

	err := ValidateInstance(global, &instance)
	if err == nil {
		t.Fatal("expected invalid transport error")
	}
	if !strings.Contains(err.Error(), "mkcp") {
		t.Fatalf("expected transport type in error: %v", err)
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
