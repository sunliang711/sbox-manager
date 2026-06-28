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
