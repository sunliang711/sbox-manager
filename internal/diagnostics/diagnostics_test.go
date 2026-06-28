package diagnostics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
	instancemgr "github.com/sunliang711/sbox-manager/internal/instance"
)

// TestSelectInstanceProxyPrefersSocks 验证 ipinfo 优先选择 socks5 listener 且摘要不泄露密码。
func TestSelectInstanceProxyPrefersSocks(t *testing.T) {
	instance := domain.Instance{
		Name: "edge-us",
		Inbounds: []domain.Inbound{
			{
				Name:   "local-http",
				Type:   "http",
				Listen: "127.0.0.1",
				Port:   18080,
			},
			{
				Name:   "local-socks",
				Type:   "socks5",
				Listen: "0.0.0.0",
				Port:   17080,
				Auth: domain.AuthConfig{
					Type:     "password",
					Username: "alice",
					Password: "secret-password",
				},
			},
		},
	}

	selected, err := SelectInstanceProxy(instance)
	if err != nil {
		t.Fatalf("select proxy: %v", err)
	}
	if selected.Scheme != "socks5" {
		t.Fatalf("expected socks5, got %s", selected.Scheme)
	}
	if selected.Address != "127.0.0.1:17080" {
		t.Fatalf("unexpected address: %s", selected.Address)
	}
	if strings.Contains(selected.Redacted().String(), "secret-password") {
		t.Fatalf("proxy summary leaked password: %s", selected.Redacted().String())
	}
	if strings.Contains(selected.Redacted().String(), "alice") {
		t.Fatalf("proxy summary leaked username: %s", selected.Redacted().String())
	}
}

// TestLookupIPInfoUsesHTTPProxyAndFallback 验证 ipinfo 通过 HTTP proxy 请求并执行 endpoint fallback。
func TestLookupIPInfoUsesHTTPProxyAndFallback(t *testing.T) {
	var calls int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"ip":"203.0.113.7"}`))
	}))
	defer proxyServer.Close()
	host, port := mustSplitTestURL(t, proxyServer.URL)
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	instance := domain.Instance{
		Name: "edge-us",
		Inbounds: []domain.Inbound{
			{
				Name:   "local-http",
				Type:   "http",
				Listen: host,
				Port:   portNumber,
			},
		},
	}
	results, err := LookupIPInfo(context.Background(), instance, IPInfoOptions{
		Family:        FamilyIPv4,
		Timeout:       2 * time.Second,
		IPv4Endpoints: []string{"http://example.invalid/first", "http://example.invalid/second"},
	})
	if err != nil {
		t.Fatalf("lookup ipinfo: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 proxy calls, got %d", got)
	}
	if len(results) != 1 || results[0].IP != "203.0.113.7" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[0].Endpoint != "http://example.invalid/second" {
		t.Fatalf("fallback endpoint not recorded: %+v", results[0])
	}
}

// TestCheckSingBoxExecutesCommand 验证 doctor 会实际调用 sing-box check。
func TestCheckSingBoxExecutesCommand(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sing-box.args")
	binary := filepath.Join(dir, "sing-box")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$*\" > %s\nexit 7\n", strconv.Quote(logPath))
	if err := os.WriteFile(binary, []byte(script), 0750); err != nil {
		t.Fatalf("write fake sing-box: %v", err)
	}

	checks := checkSingBox(context.Background(), binary, testAgentConfigSet(t))
	check := findCheck(checks, "sing-box.check")
	if check.Status != StatusIssue {
		t.Fatalf("expected sing-box.check ISSUE, got %+v", check)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake sing-box args: %v", err)
	}
	if !strings.Contains(string(data), "check -c") {
		t.Fatalf("sing-box check was not executed: %s", data)
	}
}

// TestAgentDoctorAfterInitHasNoIssue 验证刚 init 的空 agent 环境不会把未安装组件误判为故障。
func TestAgentDoctorAfterInitHasNoIssue(t *testing.T) {
	baseDir := t.TempDir()
	if err := instancemgr.Init(baseDir, instancemgr.InitOptions{}); err != nil {
		t.Fatalf("init agent: %v", err)
	}

	checks := AgentDoctor(context.Background(), baseDir, "systemd")
	if HasIssue(checks) {
		t.Fatalf("init-only doctor should not report ISSUE: %+v", checks)
	}
	if check := findCheck(checks, "sing-box.binary"); check.Status != StatusOK {
		t.Fatalf("expected sing-box.binary OK, got %+v", check)
	}
	if check := findCheck(checks, "traffic.db"); check.Status != StatusOK {
		t.Fatalf("expected traffic.db OK, got %+v", check)
	}
}

func mustSplitTestURL(t *testing.T, rawURL string) (string, string) {
	t.Helper()

	withoutScheme := strings.TrimPrefix(rawURL, "http://")
	host, port, err := net.SplitHostPort(withoutScheme)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	return host, port
}

func testAgentConfigSet(t *testing.T) *config.AgentConfigSet {
	t.Helper()

	baseDir := t.TempDir()
	global := domain.DefaultGlobalConfig()
	if err := config.NormalizeGlobalPaths(baseDir, &global); err != nil {
		t.Fatalf("normalize global paths: %v", err)
	}
	instance := domain.DefaultInstance(global)
	instance.Name = "edge-us"
	inbound := domain.DefaultInbound("vmess-main", "vmess")
	inbound.Port = 24100
	inbound.Users = []domain.InboundUser{
		{Name: "alice", UUID: "11111111-1111-4111-8111-111111111111"},
	}
	instance.Inbounds = []domain.Inbound{inbound}
	instance.Outbounds = []domain.Outbound{{Name: "direct", Type: "direct"}}
	instance.Route = domain.RouteConfig{Default: "direct"}
	if err := domain.ValidateConfigSet(global, []domain.Instance{instance}); err != nil {
		t.Fatalf("validate config set: %v", err)
	}
	return &config.AgentConfigSet{
		BaseDir:   baseDir,
		Global:    global,
		Instances: []domain.Instance{instance},
	}
}

func findCheck(checks []Check, module string) Check {
	for _, check := range checks {
		if check.Module == module {
			return check
		}
	}
	return Check{}
}
