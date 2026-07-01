package subserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/sunliang711/sbox-manager/internal/domain"
	"github.com/sunliang711/sbox-manager/internal/subscription"
)

// TestSubscriptionHTTPAuthAndErrors 覆盖订阅 HTTP 鉴权、用户不存在和模板错误。
func TestSubscriptionHTTPAuthAndErrors(t *testing.T) {
	baseDir := writeSubserverFixture(t)
	server := newTestServer(t, baseDir, domain.SubConfig{
		Version:      1,
		Listen:       "127.0.0.1:0",
		TemplatesDir: filepath.Join(baseDir, "templates"),
		Access: domain.AccessConfig{
			Type:  "token",
			Token: "good-token",
		},
		WatchInterval: 50 * time.Millisecond,
		WatchDebounce: 10 * time.Millisecond,
		ManagedConfig: domain.ManagedConfig{
			Enabled:  true,
			Interval: 86400,
			Strict:   true,
		},
	})
	handler := server.Handler()

	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantBody   string
	}{
		{name: "missing token", target: "/sub/alice", wantStatus: http.StatusUnauthorized, wantBody: "unauthorized"},
		{name: "wrong token", target: "/sub/alice?token=bad-token", wantStatus: http.StatusForbidden, wantBody: "forbidden"},
		{name: "path token wins over query", target: "/sub/good-token/alice?token=bad-token", wantStatus: http.StatusOK, wantBody: "US VMess"},
		{name: "missing user", target: "/sub/good-token/bob", wantStatus: http.StatusNotFound, wantBody: "user_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.target, nil))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status got %d want %d body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body missing %q: %s", tt.wantBody, recorder.Body.String())
			}
		})
	}

	if err := os.MkdirAll(filepath.Join(baseDir, "templates", "sub"), 0750); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "templates", "sub", "clash.yaml.j2"), []byte(`{{ .MissingField }}`), 0640); err != nil {
		t.Fatalf("write bad template: %v", err)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sub/good-token/alice", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status got %d want 503 body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "template_error") {
		t.Fatalf("expected template_error body, got %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "US VMess") {
		t.Fatalf("template error should not output subscription body: %s", recorder.Body.String())
	}
}

// TestWatcherReloadFailureKeepsOldIndex 验证 reload 失败时保留旧 index。
func TestWatcherReloadFailureKeepsOldIndex(t *testing.T) {
	baseDir := writeSubserverFixture(t)
	server := newTestServer(t, baseDir, domain.SubConfig{
		Version:       1,
		Listen:        "127.0.0.1:0",
		TemplatesDir:  filepath.Join(baseDir, "templates"),
		Access:        domain.AccessConfig{Type: "none"},
		WatchInterval: 20 * time.Millisecond,
		WatchDebounce: 5 * time.Millisecond,
		ManagedConfig: domain.ManagedConfig{
			Enabled:  true,
			Interval: 86400,
			Strict:   true,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.watchInputs(ctx)
	time.Sleep(60 * time.Millisecond)

	badInput := strings.Replace(validSubscriptionInputJSON("edge-us", "alice", "US VMess"), `"uuid":"11111111-1111-4111-8111-111111111111"`, `"uuid":""`, 1)
	if err := os.WriteFile(filepath.Join(baseDir, "inputs", "edge-us.json"), []byte(badInput), 0640); err != nil {
		t.Fatalf("write bad input: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		index, lastError := server.state.Snapshot()
		if lastError != "" {
			nodes := index.NodesForUser("alice")
			if len(nodes) != 1 || nodes[0].Remark != "US VMess" {
				t.Fatalf("old index not preserved: nodes=%+v", nodes)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected reload error")
}

// TestWatcherReloadRecoveryClearsLastError 验证 input 恢复后 health 不再保持 degraded。
func TestWatcherReloadRecoveryClearsLastError(t *testing.T) {
	baseDir := writeSubserverFixture(t)
	server := newTestServer(t, baseDir, domain.SubConfig{
		Version:       1,
		Listen:        "127.0.0.1:0",
		TemplatesDir:  filepath.Join(baseDir, "templates"),
		Access:        domain.AccessConfig{Type: "none"},
		WatchInterval: 20 * time.Millisecond,
		WatchDebounce: 5 * time.Millisecond,
		ManagedConfig: domain.ManagedConfig{
			Enabled:  true,
			Interval: 86400,
			Strict:   true,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.watchInputs(ctx)
	time.Sleep(60 * time.Millisecond)

	inputPath := filepath.Join(baseDir, "inputs", "edge-us.json")
	validInput := validSubscriptionInputJSON("edge-us", "alice", "US VMess")
	badInput := strings.Replace(validInput, `"uuid":"11111111-1111-4111-8111-111111111111"`, `"uuid":""`, 1)
	if err := os.WriteFile(inputPath, []byte(badInput), 0640); err != nil {
		t.Fatalf("write bad input: %v", err)
	}
	waitForLastError(t, server, true)

	if err := os.WriteFile(inputPath, []byte(validInput), 0640); err != nil {
		t.Fatalf("restore input: %v", err)
	}
	waitForLastError(t, server, false)
}

// TestNativeNodeOnlyRendersForSingBox 验证 native 节点不会污染 Clash/Surge 输出。
func TestNativeNodeOnlyRendersForSingBox(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(subscription.InputsDir(baseDir), 0750); err != nil {
		t.Fatalf("mkdir inputs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "inputs", "native.json"), []byte(nativeSubscriptionInputJSON()), 0640); err != nil {
		t.Fatalf("write input: %v", err)
	}
	server := newTestServer(t, baseDir, domain.SubConfig{
		Version:       1,
		Listen:        "127.0.0.1:0",
		TemplatesDir:  filepath.Join(baseDir, "templates"),
		Access:        domain.AccessConfig{Type: "none"},
		WatchInterval: 50 * time.Millisecond,
		WatchDebounce: 10 * time.Millisecond,
		ManagedConfig: domain.ManagedConfig{
			Enabled:  true,
			Interval: 86400,
			Strict:   true,
		},
	})
	handler := server.Handler()

	clashRecorder := httptest.NewRecorder()
	handler.ServeHTTP(clashRecorder, httptest.NewRequest(http.MethodGet, "/sub/alice", nil))
	if clashRecorder.Code != http.StatusNotFound {
		t.Fatalf("clash native-only status got %d want 404 body=%s", clashRecorder.Code, clashRecorder.Body.String())
	}

	singBoxRecorder := httptest.NewRecorder()
	handler.ServeHTTP(singBoxRecorder, httptest.NewRequest(http.MethodGet, "/sing-box/alice", nil))
	if singBoxRecorder.Code != http.StatusOK {
		t.Fatalf("sing-box native status got %d want 200 body=%s", singBoxRecorder.Code, singBoxRecorder.Body.String())
	}
	if !strings.Contains(singBoxRecorder.Body.String(), `"type": "trojan"`) {
		t.Fatalf("sing-box output missing native outbound: %s", singBoxRecorder.Body.String())
	}
}

// newTestServer 创建测试用 server。
func newTestServer(t *testing.T, baseDir string, config domain.SubConfig) *Server {
	t.Helper()

	var logs bytes.Buffer
	server, err := New(Options{
		BaseDir: baseDir,
		Config:  config,
		Logger:  zerolog.New(&logs),
		Now: func() time.Time {
			return time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return server
}

// writeSubserverFixture 写入订阅 HTTP 测试所需的 input。
func writeSubserverFixture(t *testing.T) string {
	t.Helper()

	baseDir := t.TempDir()
	if err := os.MkdirAll(subscription.InputsDir(baseDir), 0750); err != nil {
		t.Fatalf("mkdir inputs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "inputs", "edge-us.json"), []byte(validSubscriptionInputJSON("edge-us", "alice", "US VMess")), 0640); err != nil {
		t.Fatalf("write input: %v", err)
	}
	return baseDir
}

// validSubscriptionInputJSON 返回一个合法订阅 input JSON。
func validSubscriptionInputJSON(source string, user string, remark string) string {
	value := map[string]interface{}{
		"input_schema":  "sbox.subscription-input",
		"input_version": 1,
		"source":        source,
		"generated_at":  "2026-06-28T12:00:00Z",
		"external_host": "proxy.example.com",
		"nodes": []map[string]interface{}{
			{
				"id":       source + ":" + user + ":vmess-main",
				"user":     user,
				"protocol": "vmess",
				"server":   "proxy.example.com",
				"port":     24100,
				"tag":      source + "-vmess-main",
				"remark":   remark,
				"region":   "US",
				"uuid":     "11111111-1111-4111-8111-111111111111",
				"network":  "tcp",
			},
		},
	}
	data, _ := json.Marshal(value)
	return string(data)
}

// nativeSubscriptionInputJSON 返回只有 sing-box native 节点的 input。
func nativeSubscriptionInputJSON() string {
	value := map[string]interface{}{
		"input_schema":  "sbox.subscription-input",
		"input_version": 1,
		"source":        "native",
		"generated_at":  "2026-06-28T12:00:00Z",
		"external_host": "proxy.example.com",
		"nodes": []map[string]interface{}{
			{
				"id":       "native:alice:trojan-main",
				"user":     "alice",
				"protocol": "sing-box",
				"server":   "proxy.example.com",
				"port":     443,
				"tag":      "native-trojan-main",
				"remark":   "Native Trojan",
				"native": map[string]interface{}{
					"type":        "trojan",
					"tag":         "native-trojan-main",
					"server":      "proxy.example.com",
					"server_port": 443,
					"password":    "secret",
				},
			},
		},
	}
	data, _ := json.Marshal(value)
	return string(data)
}

// waitForLastError 等待 watcher 进入或退出 reload 错误状态。
func waitForLastError(t *testing.T, server *Server, want bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, lastError := server.state.Snapshot()
		if (lastError != "") == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("last_error state did not become %t", want)
}
