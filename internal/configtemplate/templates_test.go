package configtemplate

import (
	"strings"
	"testing"
)

// TestHTTPUpgradeTransportSnippetSkipsUnsupportedMethod 验证 HTTPUpgrade 模板不暴露 sing-box 不支持的 method 字段。
func TestHTTPUpgradeTransportSnippetSkipsUnsupportedMethod(t *testing.T) {
	output, err := RenderSnippet("transport", "httpupgrade", DefaultContext())
	if err != nil {
		t.Fatalf("render httpupgrade transport snippet: %v", err)
	}
	if !strings.Contains(output, "type: httpupgrade") || !strings.Contains(output, "path: /upgrade") {
		t.Fatalf("httpupgrade transport snippet missing expected fields:\n%s", output)
	}
	if strings.Contains(output, "\n  method:") {
		t.Fatalf("httpupgrade transport snippet should not contain active method field:\n%s", output)
	}
}

// TestVMessQUICInboundSnippetIncludesTLS 验证 VMess QUIC 入站模板包含 QUIC 必需的 TLS 示例。
func TestVMessQUICInboundSnippetIncludesTLS(t *testing.T) {
	output, err := RenderSnippet("inbound", "vmess-quic", DefaultContext())
	if err != nil {
		t.Fatalf("render vmess quic inbound snippet: %v", err)
	}
	for _, want := range []string{"# VMess QUIC inbound", "tls:", "enabled: true", "alpn: [h3]", "certificate_path:", "key_path:", "type: quic"} {
		if !strings.Contains(output, want) {
			t.Fatalf("vmess quic inbound snippet missing %q:\n%s", want, output)
		}
	}
}
