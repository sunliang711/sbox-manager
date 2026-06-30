package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestRemoteSourceRequiresSHA256 验证自定义远端 source 必须显式提供 sha256。
func TestRemoteSourceRequiresSHA256(t *testing.T) {
	global := installerFixtureGlobal(t)
	err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceSingBox,
		Source:    "https://example.com/sing-box.tar.gz",
	})
	if err == nil {
		t.Fatal("expected missing sha256 error")
	}
	if !strings.Contains(err.Error(), "--sha256") {
		t.Fatalf("expected sha256 error, got %v", err)
	}
}

// TestDefaultSourcesHaveTrustedChecksums 验证默认内置 source 具备可信 checksum 元数据。
func TestDefaultSourcesHaveTrustedChecksums(t *testing.T) {
	installer := NewInstaller()
	singBox, err := installer.resolveSource(Options{Resource: ResourceSingBox})
	if err != nil {
		t.Fatalf("resolve built-in sing-box source: %v", err)
	}
	if singBox.URL == "" || singBox.SHA256 == "" {
		t.Fatalf("built-in sing-box source missing metadata: %+v", singBox)
	}
	rules, err := installer.resolveSource(Options{Resource: ResourceRules})
	if err != nil {
		t.Fatalf("resolve built-in rules source: %v", err)
	}
	if len(rules.Files) == 0 {
		t.Fatalf("built-in rules source should use trusted files: %+v", rules)
	}
	for _, file := range rules.Files {
		if file.URL == "" || file.SHA256 == "" || file.Path == "" {
			t.Fatalf("built-in rules file missing metadata: %+v", file)
		}
	}
}

// TestArchiveTraversalRejected 验证归档成员路径穿越会被拒绝。
func TestArchiveTraversalRejected(t *testing.T) {
	global := installerFixtureGlobal(t)
	archive := tarGz(t, map[string][]byte{
		"rules/geosite.db": []byte("ok"),
		"../evil":          []byte("bad"),
	})
	source := writeSource(t, archive)

	err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceRules,
		Source:    source,
	})
	if err == nil {
		t.Fatal("expected traversal error")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("expected unsafe path error, got %v", err)
	}
}

// TestInstallRulesNormalizesDirectoryMetadata 验证 rules 替换后目录权限保持服务可访问。
func TestInstallRulesNormalizesDirectoryMetadata(t *testing.T) {
	global := installerFixtureGlobal(t)
	archive := tarGz(t, map[string][]byte{
		"nested/geosite.db": []byte("ok"),
	})
	source := writeSource(t, archive)

	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceRules,
		Source:    source,
	}); err != nil {
		t.Fatalf("install rules: %v", err)
	}
	for _, path := range []string{
		global.Paths.Rules,
		filepath.Join(global.Paths.Rules, "nested"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat rules dir %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0750 {
			t.Fatalf("rules dir %s mode = %o, want 0750", path, got)
		}
	}
}

// TestInstallSingBoxWithSHA256AndArchiveMember 验证 sha256 和 archive member 安装路径。
func TestInstallSingBoxWithSHA256AndArchiveMember(t *testing.T) {
	global := installerFixtureGlobal(t)
	payload := []byte("#!/bin/sh\necho sing-box\n")
	archive := tarGz(t, map[string][]byte{
		"sing-box-1.0/sing-box": payload,
	})
	source := writeSource(t, archive)

	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation:     OperationInstall,
		Resource:      ResourceSingBox,
		Source:        source,
		SHA256:        shaHex(archive),
		ArchiveMember: "sing-box-1.0/sing-box",
	}); err != nil {
		t.Fatalf("install sing-box: %v", err)
	}

	target := filepath.Join(global.Paths.Bin, "sing-box")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("unexpected payload: %q", data)
	}
	if _, err := os.Stat(filepath.Join(global.Paths.Bin, singBoxManagedMarker)); err != nil {
		t.Fatalf("missing managed marker: %v", err)
	}
}

// TestInstallSingBoxSkipsMatchingManagedSource 验证已安装相同 source hash 时不重复读取或下载。
func TestInstallSingBoxSkipsMatchingManagedSource(t *testing.T) {
	global := installerFixtureGlobal(t)
	payload := []byte("#!/bin/sh\necho sing-box\n")
	source := writeSource(t, payload)
	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceSingBox,
		Source:    source,
		SHA256:    shaHex(payload),
	}); err != nil {
		t.Fatalf("first install sing-box: %v", err)
	}
	progress := []string{}
	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceSingBox,
		Source:    source,
		SHA256:    shaHex(payload),
		Progress: func(message string) {
			progress = append(progress, message)
		},
	}); err != nil {
		t.Fatalf("second install sing-box: %v", err)
	}
	joined := strings.Join(progress, "\n")
	if !strings.Contains(joined, "install: skip sing-box already installed") {
		t.Fatalf("second install should skip existing sing-box:\n%s", joined)
	}
	if strings.Contains(joined, "source: read local") {
		t.Fatalf("second install should not read source again:\n%s", joined)
	}
}

// TestInstallRulesSkipsMatchingManagedFiles 验证 geosite/geoip 文件 hash 匹配时跳过安装。
func TestInstallRulesSkipsMatchingManagedFiles(t *testing.T) {
	global := installerFixtureGlobal(t)
	geosite := []byte("geosite")
	geoip := []byte("geoip")
	geositeSource := writeSource(t, geosite)
	geoipSource := writeSource(t, geoip)
	installer := &Installer{
		Client: http.DefaultClient,
		Sources: map[string]Source{
			ResourceRules: {
				Files: []SourceFile{
					{URL: geositeSource, SHA256: shaHex(geosite), Path: "geosite.db"},
					{URL: geoipSource, SHA256: shaHex(geoip), Path: "geoip.db"},
				},
			},
		},
	}
	if err := installer.Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceRules,
	}); err != nil {
		t.Fatalf("first install rules: %v", err)
	}
	progress := []string{}
	if err := installer.Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceRules,
		Progress: func(message string) {
			progress = append(progress, message)
		},
	}); err != nil {
		t.Fatalf("second install rules: %v", err)
	}
	joined := strings.Join(progress, "\n")
	if !strings.Contains(joined, "install: skip rules already installed") {
		t.Fatalf("second install should skip existing rules:\n%s", joined)
	}
	if strings.Contains(joined, "source: read local") {
		t.Fatalf("second install should not read rule sources again:\n%s", joined)
	}
}

// TestRemoteDownloadEmitsProgress 验证远端下载会持续输出下载进度。
func TestRemoteDownloadEmitsProgress(t *testing.T) {
	global := installerFixtureGlobal(t)
	payload := bytes.Repeat([]byte("x"), 128*1024)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		if _, err := writer.Write(payload); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer server.Close()
	progress := []string{}

	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceSingBox,
		Source:    server.URL + "/sing-box",
		SHA256:    shaHex(payload),
		Progress: func(message string) {
			progress = append(progress, message)
		},
	}); err != nil {
		t.Fatalf("install remote sing-box: %v", err)
	}
	joined := strings.Join(progress, "\n")
	if !strings.Contains(joined, "download: progress") || !strings.Contains(joined, "100%") {
		t.Fatalf("download progress missing:\n%s", joined)
	}
}

// TestInstallerEmitsProgress 验证安装器会输出可见的阶段性进度。
func TestInstallerEmitsProgress(t *testing.T) {
	global := installerFixtureGlobal(t)
	payload := []byte("#!/bin/sh\necho sing-box\n")
	source := writeSource(t, payload)
	progress := []string{}

	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceSingBox,
		Source:    source,
		SHA256:    shaHex(payload),
		Progress: func(message string) {
			progress = append(progress, message)
		},
	}); err != nil {
		t.Fatalf("install sing-box: %v", err)
	}

	joined := strings.Join(progress, "\n")
	for _, want := range []string{
		"install: start sing-box",
		"install: prepare sing-box",
		"source: resolved sing-box",
		"source: read local",
		"verify: sha256",
		"install: extract sing-box",
		"install: write",
		"install: complete sing-box",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("progress missing %q:\n%s", want, joined)
		}
	}
}

// TestFailedUpdatePreservesOldSingBox 验证更新失败不会破坏旧二进制。
func TestFailedUpdatePreservesOldSingBox(t *testing.T) {
	global := installerFixtureGlobal(t)
	oldSource := writeSource(t, []byte("old"))
	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceSingBox,
		Source:    oldSource,
	}); err != nil {
		t.Fatalf("install old: %v", err)
	}

	badSource := writeSource(t, []byte("new"))
	err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationUpdate,
		Resource:  ResourceSingBox,
		Source:    badSource,
		SHA256:    strings.Repeat("0", sha256.Size*2),
	})
	if err == nil {
		t.Fatal("expected update sha mismatch")
	}
	data, err := os.ReadFile(filepath.Join(global.Paths.Bin, "sing-box"))
	if err != nil {
		t.Fatalf("read old binary: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("old binary should be preserved, got %q", data)
	}
}

// TestUnmanagedSymlinkNotOverwritten 验证unmanaged symlink 不会被覆盖。
func TestUnmanagedSymlinkNotOverwritten(t *testing.T) {
	global := installerFixtureGlobal(t)
	if err := os.MkdirAll(global.Paths.Bin, 0750); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	external := filepath.Join(t.TempDir(), "external-sing-box")
	if err := os.WriteFile(external, []byte("external"), 0755); err != nil {
		t.Fatalf("write external: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(global.Paths.Bin, "sing-box")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationInstall,
		Resource:  ResourceSingBox,
		Source:    writeSource(t, []byte("managed")),
	})
	if err == nil {
		t.Fatal("expected unmanaged symlink error")
	}
	if !strings.Contains(err.Error(), "unmanaged symlink") {
		t.Fatalf("expected unmanaged symlink error, got %v", err)
	}
}

// TestUninstallPurgeRemovesManagedResourceDirs 验证 purge 会删除资源安装器负责的受管目录。
func TestUninstallPurgeRemovesManagedResourceDirs(t *testing.T) {
	global := installerFixtureGlobal(t)
	for _, path := range []string{global.Paths.Bin, global.Paths.Rules, global.Paths.Downloads, global.Paths.Runtime} {
		if err := os.MkdirAll(path, 0750); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "managed"), []byte("data"), 0640); err != nil {
			t.Fatalf("write managed file: %v", err)
		}
	}
	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationUninstall,
		Resource:  ResourceAll,
		Purge:     true,
	}); err != nil {
		t.Fatalf("uninstall purge: %v", err)
	}
	for _, path := range []string{global.Paths.Bin, global.Paths.Rules, global.Paths.Downloads, global.Paths.Runtime} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("managed dir should be removed %s, stat err: %v", path, err)
		}
	}
}

// TestUninstallPurgeSkipsMissingManagedResourceDirs 验证 purge 遇到已不存在的受管目录时保持幂等。
func TestUninstallPurgeSkipsMissingManagedResourceDirs(t *testing.T) {
	global := installerFixtureGlobal(t)

	if err := NewInstaller().Run(context.Background(), global, Options{
		Operation: OperationUninstall,
		Resource:  ResourceAll,
		Purge:     true,
	}); err != nil {
		t.Fatalf("uninstall purge with missing dirs: %v", err)
	}
}

func installerFixtureGlobal(t *testing.T) domain.GlobalConfig {
	t.Helper()
	baseDir := t.TempDir()
	global := domain.DefaultGlobalConfig()
	global.Paths.Bin = filepath.Join(baseDir, "bin")
	global.Paths.Rules = filepath.Join(baseDir, "rules")
	global.Paths.Instances = filepath.Join(baseDir, "instances")
	global.Paths.Runtime = filepath.Join(baseDir, "runtime")
	global.Paths.Generated = filepath.Join(baseDir, "runtime", "generated")
	global.Paths.Publish = filepath.Join(baseDir, "publish")
	global.Paths.Traffic = filepath.Join(baseDir, "traffic")
	global.Paths.Downloads = filepath.Join(baseDir, "downloads")
	global.Paths.Logs = filepath.Join(baseDir, "logs")
	return global
}

func writeSource(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(path, data, 0640); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return path
}

func tarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, data := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(data))}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			t.Fatalf("write data: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func shaHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
