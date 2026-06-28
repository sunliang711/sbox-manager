package singbox

import (
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
	assertOrder(t, output, `"log"`, `"dns"`)
	assertOrder(t, output, `"dns"`, `"inbounds"`)
	assertOrder(t, output, `"inbounds"`, `"outbounds"`)
	assertOrder(t, output, `"outbounds"`, `"route"`)
	assertOrder(t, output, `"route"`, `"experimental"`)
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
