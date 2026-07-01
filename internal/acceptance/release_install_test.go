package acceptance

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseWorkflowStaticContract 验证 release workflow 的 tag、权限、矩阵和资产上传契约。
func TestReleaseWorkflowStaticContract(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"tags:",
		`"v*.*.*"`,
		"contents: write",
		"Tag ${GITHUB_REF_NAME} is not a semantic release tag",
		"make test",
		"build_commit=\"$(git rev-parse --short HEAD)\"",
		"build_time=\"$(date -u '+%Y-%m-%dT%H:%M:%SZ')\"",
		"make package GOOS=\"${{ matrix.goos }}\" GOARCH=\"${{ matrix.goarch }}\" VERSION=\"${GITHUB_REF_NAME}\" COMMIT=\"${build_commit}\" BUILD_TIME=\"${build_time}\"",
		"make checksums",
		"gh release view",
		"gh release create \"${GITHUB_REF_NAME}\" dist/release/*.tar.gz dist/release/checksums.txt",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow missing %q:\n%s", want, text)
		}
	}
	for _, want := range []string{
		"goos: linux\n            goarch: amd64",
		"goos: linux\n            goarch: arm64",
		"goos: darwin\n            goarch: amd64",
		"goos: darwin\n            goarch: arm64",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release workflow missing matrix pair %q:\n%s", want, text)
		}
	}
}

// TestInstallScriptDryRunDoesNotWrite 验证远程安装脚本 dry-run 不创建安装目标。
func TestInstallScriptDryRunDoesNotWrite(t *testing.T) {
	root := repoRoot(t)
	requireCommand(t, "bash")
	requireCommand(t, "curl")
	requireCommand(t, "tar")

	installDir := filepath.Join(t.TempDir(), "bin")
	tmpDir := filepath.Join(t.TempDir(), "tmp")
	output := runScript(t, root,
		filepath.Join(root, "scripts", "install.sh"),
		"--version", "v0.0.0",
		"--repo", "owner/repo",
		"--os", "linux",
		"--arch", "amd64",
		"--install-dir", installDir,
		"--tmp-dir", tmpDir,
		"--dry-run",
	)
	for _, want := range []string{"Would download", "Would install sboxctl and sboxsub"} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, output)
		}
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create install dir, stat err: %v", err)
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create tmp dir, stat err: %v", err)
	}
}

// TestInstallLocalDryRunDoesNotWrite 验证本地安装脚本 dry-run 不写入目标二进制。
func TestInstallLocalDryRunDoesNotWrite(t *testing.T) {
	root := repoRoot(t)
	requireCommand(t, "bash")

	sourceDir := filepath.Join(t.TempDir(), "dist", "bin")
	writeExecutable(t, filepath.Join(sourceDir, "sboxctl"))
	writeExecutable(t, filepath.Join(sourceDir, "sboxsub"))
	installDir := filepath.Join(t.TempDir(), "bin")

	output := runScript(t, root,
		filepath.Join(root, "scripts", "install-local.sh"),
		"--from", sourceDir,
		"--install-dir", installDir,
		"--dry-run",
	)
	for _, want := range []string{"Would install " + filepath.Join(installDir, "sboxctl"), "Would install " + filepath.Join(installDir, "sboxsub")} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, output)
		}
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create install dir, stat err: %v", err)
	}
}

// TestInstallLocalRejectsUnsafeArchive 验证本地安装脚本拒绝路径穿越归档且不写入目标。
func TestInstallLocalRejectsUnsafeArchive(t *testing.T) {
	root := repoRoot(t)
	requireCommand(t, "bash")
	requireCommand(t, "tar")

	archivePath := filepath.Join(t.TempDir(), "unsafe.tar.gz")
	writeTarGz(t, archivePath, map[string]string{
		"sbox-manager_v0.0.0_linux_amd64/bin/sboxctl": "ok",
		"../evil": "bad",
	})
	installDir := filepath.Join(t.TempDir(), "bin")
	command := exec.Command("bash", filepath.Join(root, "scripts", "install-local.sh"), "--from", archivePath, "--install-dir", installDir)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("expected unsafe archive failure, output=%s", output)
	}
	if !strings.Contains(string(output), "Unsafe archive member") {
		t.Fatalf("unexpected unsafe archive output:\n%s", output)
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("unsafe archive should not create install dir, stat err: %v", err)
	}
}

// TestInstallScriptRejectsUnsafeArchive 验证远程安装脚本拒绝路径穿越归档且不写入目标。
func TestInstallScriptRejectsUnsafeArchive(t *testing.T) {
	root := repoRoot(t)
	requireCommand(t, "bash")
	requireCommand(t, "tar")

	tempDir := t.TempDir()
	assetName := "sbox-manager_v0.0.0_linux_amd64.tar.gz"
	assetPath := filepath.Join(tempDir, assetName)
	writeTarGz(t, assetPath, map[string]string{
		"sbox-manager_v0.0.0_linux_amd64/bin/sboxctl":       "ok",
		"sbox-manager_v0.0.0_linux_amd64/templates/../evil": "bad",
	})
	fakeBin := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(fakeBin, 0750); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	writeFakeCurl(t, filepath.Join(fakeBin, "curl"), assetPath, assetName)

	installDir := filepath.Join(tempDir, "install")
	tmpDir := filepath.Join(tempDir, "tmp")
	command := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"),
		"--version", "v0.0.0",
		"--repo", "owner/repo",
		"--os", "linux",
		"--arch", "amd64",
		"--install-dir", installDir,
		"--tmp-dir", tmpDir,
		"--no-checksum",
	)
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("expected unsafe archive failure, output=%s", output)
	}
	if !strings.Contains(string(output), "Unsafe archive member") {
		t.Fatalf("unexpected unsafe archive output:\n%s", output)
	}
	if _, err := os.Stat(installDir); !os.IsNotExist(err) {
		t.Fatalf("unsafe archive should not create install dir, stat err: %v", err)
	}
}

// TestInstallScriptChecksumMismatchDoesNotInstall 验证远程安装脚本默认校验 checksum 且失败时不安装。
func TestInstallScriptChecksumMismatchDoesNotInstall(t *testing.T) {
	root := repoRoot(t)
	requireCommand(t, "bash")
	requireCommand(t, "tar")

	tempDir := t.TempDir()
	assetName := "sbox-manager_v0.0.0_linux_amd64.tar.gz"
	assetPath := filepath.Join(tempDir, assetName)
	writeTarGz(t, assetPath, map[string]string{
		"sbox-manager_v0.0.0_linux_amd64/bin/sboxctl": "ok",
		"sbox-manager_v0.0.0_linux_amd64/bin/sboxsub": "ok",
	})
	fakeBin := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(fakeBin, 0750); err != nil {
		t.Fatalf("mkdir fake bin: %v", err)
	}
	writeFakeCurl(t, filepath.Join(fakeBin, "curl"), assetPath, assetName)

	installDir := filepath.Join(tempDir, "install")
	tmpDir := filepath.Join(tempDir, "tmp")
	command := exec.Command("bash", filepath.Join(root, "scripts", "install.sh"),
		"--version", "v0.0.0",
		"--repo", "owner/repo",
		"--os", "linux",
		"--arch", "amd64",
		"--install-dir", installDir,
		"--tmp-dir", tmpDir,
	)
	command.Dir = root
	command.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("expected checksum mismatch failure, output=%s", output)
	}
	if !strings.Contains(string(output), "checksum") && !strings.Contains(string(output), "Checksum") {
		t.Fatalf("expected checksum failure output, got:\n%s", output)
	}
	for _, name := range []string{"sboxctl", "sboxsub"} {
		if _, err := os.Stat(filepath.Join(installDir, name)); !os.IsNotExist(err) {
			t.Fatalf("checksum mismatch should not install %s, stat err: %v", name, err)
		}
	}
}

// repoRoot 从当前测试目录向上查找 go.mod，返回仓库根目录。
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}

// requireCommand 在当前环境缺少外部命令时跳过对应脚本验收。
func requireCommand(t *testing.T, name string) {
	t.Helper()

	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found", name)
	}
}

// runScript 执行脚本并返回标准输出和标准错误合并内容。
func runScript(t *testing.T, root string, script string, args ...string) string {
	t.Helper()

	command := exec.Command("bash", append([]string{script}, args...)...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s: %v\n%s", script, err, output)
	}
	return string(output)
}

// writeExecutable 写入测试用可执行文件。
func writeExecutable(t *testing.T, target string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		t.Fatalf("mkdir executable dir: %v", err)
	}
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}

// writeFakeCurl 写入只从本地复制 release 资产并返回错误 checksum 的 curl 替身。
func writeFakeCurl(t *testing.T, target string, assetPath string, assetName string) {
	t.Helper()

	script := fmt.Sprintf(`#!/bin/sh
out=""
url=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  url="$1"
  shift
done
case "$url" in
  *checksums.txt)
    printf '0000000000000000000000000000000000000000000000000000000000000000  %s\n' > "$out"
    ;;
  *)
    cp %s "$out"
    ;;
esac
`, assetName, shellQuote(assetPath))
	if err := os.WriteFile(target, []byte(script), 0755); err != nil {
		t.Fatalf("write fake curl: %v", err)
	}
}

// writeTarGz 写入带指定成员的 tar.gz 测试归档。
func writeTarGz(t *testing.T, target string, members map[string]string) {
	t.Helper()

	file, err := os.Create(target)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()
	for name, body := range members {
		header := &tar.Header{
			Name: name,
			Mode: 0755,
			Size: int64(len(body)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %s: %v", name, err)
		}
		if _, err := tarWriter.Write([]byte(body)); err != nil {
			t.Fatalf("write tar body %s: %v", name, err)
		}
	}
}

// shellQuote 返回可嵌入 POSIX shell 脚本的单引号字符串。
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
