package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

const (
	ResourceSingBox = "sing-box"
	ResourceRules   = "rules"
	ResourceAll     = "all"

	OperationInstall   = "install"
	OperationUpdate    = "update"
	OperationUninstall = "uninstall"
)

const (
	singBoxManagedMarker = ".sing-box.managed"
	rulesManagedMarker   = ".sbox-manager-managed"

	downloadProgressStepBytes = 1 << 20
)

// Source 描述受信任的内置下载源。
type Source struct {
	URL           string
	SHA256        string
	ArchiveMember string
	Files         []SourceFile
}

// SourceFile 描述内置 multi-file source 中的单个受信任文件。
type SourceFile struct {
	URL    string
	SHA256 string
	Path   string
}

// Progress 接收安装过程中的阶段性英文日志消息。
type Progress func(message string)

// Options 描述一次资源安装、更新或卸载操作。
type Options struct {
	Operation     string
	Resource      string
	Version       string
	Source        string
	SHA256        string
	ArchiveMember string
	Purge         bool
	Progress      Progress
}

// Installer 负责 sing-box 和规则资源的安全安装。
type Installer struct {
	Client  *http.Client
	Sources map[string]Source
}

// NewInstaller 创建默认安装器。
func NewInstaller() *Installer {
	return &Installer{
		Client:  http.DefaultClient,
		Sources: defaultSources(),
	}
}

// Run 执行资源操作。
func (i *Installer) Run(ctx context.Context, global domain.GlobalConfig, options Options) error {
	resources, err := expandResources(options.Resource)
	if err != nil {
		return err
	}
	emitProgress(options.Progress, "%s: start %s", options.Operation, options.Resource)
	for _, resource := range resources {
		next := options
		next.Resource = resource
		switch options.Operation {
		case OperationInstall, OperationUpdate:
			if err := i.installOne(ctx, global, next); err != nil {
				return err
			}
		case OperationUninstall:
			if err := i.uninstallOne(global, next); err != nil {
				return err
			}
		default:
			return fmt.Errorf("不支持的安装动作 %q", options.Operation)
		}
	}
	if options.Operation == OperationUninstall && options.Purge {
		emitProgress(options.Progress, "%s: purge managed directories", options.Operation)
		if err := PurgeManaged(global); err != nil {
			return err
		}
		emitProgress(options.Progress, "%s: purged managed directories", options.Operation)
	}
	if options.Resource == ResourceAll {
		emitProgress(options.Progress, "%s: complete %s", options.Operation, options.Resource)
	}
	return nil
}

// PurgeManaged 删除资源安装器负责的受管目录，保留 config、instances 和 traffic 数据。
func PurgeManaged(global domain.GlobalConfig) error {
	for _, path := range []string{
		global.Paths.Bin,
		global.Paths.Rules,
		global.Paths.Downloads,
		global.Paths.Runtime,
	} {
		exists, err := pathExists(path)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("清理受管目录 %s: %w", path, err)
		}
	}
	return nil
}

// pathExists 使用 Lstat 判断受管资源路径是否存在，路径不存在时返回 false。
func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("检查路径 %s: %w", path, err)
	}
	return true, nil
}

func expandResources(resource string) ([]string, error) {
	switch resource {
	case ResourceSingBox:
		return []string{ResourceSingBox}, nil
	case ResourceRules:
		return []string{ResourceRules}, nil
	case ResourceAll:
		return []string{ResourceSingBox, ResourceRules}, nil
	default:
		return nil, fmt.Errorf("不支持的资源 %q", resource)
	}
}

// emitProgress 输出安装进度事件，供 CLI 层展示给用户。
func emitProgress(progress Progress, format string, args ...interface{}) {
	if progress == nil {
		return
	}
	progress(fmt.Sprintf(format, args...))
}

// emitResolvedSource 输出解析后的 source 摘要，避免泄露 URL userinfo 和 query。
func emitResolvedSource(progress Progress, resource string, source Source) {
	if len(source.Files) == 0 {
		emitProgress(progress, "source: resolved %s %s", resource, safeSourceSummary(source.URL))
		return
	}
	emitProgress(progress, "source: resolved %s files=%d", resource, len(source.Files))
	for _, file := range source.Files {
		emitProgress(progress, "source: file %s %s", file.Path, safeSourceSummary(file.URL))
	}
}

// safeSourceSummary 返回适合日志展示的 source 摘要。
func safeSourceSummary(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme == "" {
		return source
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func (i *Installer) installOne(ctx context.Context, global domain.GlobalConfig, options Options) error {
	emitProgress(options.Progress, "%s: prepare %s", options.Operation, options.Resource)
	source, err := i.resolveSource(options)
	if err != nil {
		return err
	}
	emitResolvedSource(options.Progress, options.Resource, source)
	if options.ArchiveMember != "" {
		source.ArchiveMember = options.ArchiveMember
		emitProgress(options.Progress, "%s: use archive member %s", options.Operation, source.ArchiveMember)
	}
	switch options.Resource {
	case ResourceSingBox:
		if len(source.Files) > 0 {
			return fmt.Errorf("sing-box source 不支持 multi-file")
		}
		if options.Operation == OperationInstall && source.SHA256 != "" {
			installed, err := singBoxSourceInstalled(global.Paths.Bin, source.SHA256, source.ArchiveMember)
			if err != nil {
				return err
			}
			if installed {
				emitProgress(options.Progress, "%s: skip %s already installed", options.Operation, options.Resource)
				return nil
			}
		}
		data, err := i.fetch(ctx, global.Paths.Downloads, source.URL, source.SHA256, options.Progress)
		if err != nil {
			return err
		}
		emitProgress(options.Progress, "%s: extract sing-box", options.Operation)
		payload, err := extractSingBox(data, source.ArchiveMember)
		if err != nil {
			return err
		}
		emitProgress(options.Progress, "%s: write %s", options.Operation, filepath.Join(global.Paths.Bin, "sing-box"))
		if err := installSingBox(global.Paths.Bin, payload, hashBytes(payload), source.SHA256, source.ArchiveMember); err != nil {
			return err
		}
		emitProgress(options.Progress, "%s: complete %s", options.Operation, options.Resource)
		return nil
	case ResourceRules:
		if len(source.Files) > 0 {
			if options.Operation == OperationInstall {
				installed, err := rulesFilesInstalled(global.Paths.Rules, source.Files)
				if err != nil {
					return err
				}
				if installed {
					emitProgress(options.Progress, "%s: skip %s already installed", options.Operation, options.Resource)
					return nil
				}
			}
			if err := i.installRulesFiles(ctx, global.Paths.Downloads, global.Paths.Rules, source.Files, options.Operation, options.Progress); err != nil {
				return err
			}
			emitProgress(options.Progress, "%s: complete %s", options.Operation, options.Resource)
			return nil
		}
		data, err := i.fetch(ctx, global.Paths.Downloads, source.URL, source.SHA256, options.Progress)
		if err != nil {
			return err
		}
		emitProgress(options.Progress, "%s: extract rules", options.Operation)
		emitProgress(options.Progress, "%s: write %s", options.Operation, global.Paths.Rules)
		if err := installRules(global.Paths.Rules, data); err != nil {
			return err
		}
		emitProgress(options.Progress, "%s: complete %s", options.Operation, options.Resource)
		return nil
	default:
		return fmt.Errorf("不支持的资源 %q", options.Resource)
	}
}

func (i *Installer) resolveSource(options Options) (Source, error) {
	source := strings.TrimSpace(options.Source)
	shaText := strings.TrimSpace(options.SHA256)
	member := strings.TrimSpace(options.ArchiveMember)
	if source == "" {
		key := builtinKey(options.Resource, options.Version)
		builtin, ok := i.Sources[key]
		if !ok {
			builtin, ok = i.Sources[builtinKey(options.Resource, "")]
		}
		if !ok {
			return Source{}, fmt.Errorf("%s 未配置内置 source，请使用 --source", options.Resource)
		}
		if err := validateTrustedSource(options.Resource, builtin); err != nil {
			return Source{}, err
		}
		return builtin, nil
	}
	if isRemoteSource(source) && shaText == "" {
		return Source{}, fmt.Errorf("自定义远端 source %s 必须显式提供 --sha256", source)
	}
	return Source{URL: source, SHA256: shaText, ArchiveMember: member}, nil
}

func builtinKey(resource string, version string) string {
	if version == "" {
		return resource
	}
	return resource + "@" + version
}

func validateTrustedSource(resource string, source Source) error {
	if len(source.Files) == 0 {
		if strings.TrimSpace(source.URL) == "" {
			return fmt.Errorf("%s 内置 source 缺少 URL", resource)
		}
		if strings.TrimSpace(source.SHA256) == "" {
			return fmt.Errorf("%s 内置 source 缺少可信 sha256", resource)
		}
		return nil
	}
	for _, file := range source.Files {
		if strings.TrimSpace(file.URL) == "" {
			return fmt.Errorf("%s 内置 source 文件缺少 URL", resource)
		}
		if strings.TrimSpace(file.SHA256) == "" {
			return fmt.Errorf("%s 内置 source 文件 %s 缺少可信 sha256", resource, file.Path)
		}
		if err := validateArchiveName(file.Path); err != nil {
			return err
		}
	}
	return nil
}

func (i *Installer) fetch(ctx context.Context, downloadsDir string, source string, shaText string, progress Progress) ([]byte, error) {
	var data []byte
	var err error
	if isRemoteSource(source) {
		emitProgress(progress, "download: start %s", safeSourceSummary(source))
		data, err = i.fetchRemote(ctx, downloadsDir, source, progress)
		if err == nil {
			emitProgress(progress, "download: complete %s bytes=%d", safeSourceSummary(source), len(data))
		}
	} else {
		emitProgress(progress, "source: read local %s", source)
		data, err = os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("读取 source %s: %w", source, err)
		}
		emitProgress(progress, "source: complete local %s bytes=%d", source, len(data))
	}
	if err != nil {
		return nil, err
	}
	if shaText != "" {
		emitProgress(progress, "verify: sha256 %s", safeSourceSummary(source))
		if err := verifySHA256(data, shaText); err != nil {
			return nil, err
		}
		emitProgress(progress, "verify: passed %s", safeSourceSummary(source))
	} else {
		emitProgress(progress, "verify: skipped %s", safeSourceSummary(source))
	}
	return data, nil
}

func (i *Installer) fetchRemote(ctx context.Context, downloadsDir string, source string, progress Progress) ([]byte, error) {
	client := i.Client
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, fmt.Errorf("创建下载请求 %s: %w", source, err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("下载 source %s: %w", source, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("下载 source %s 返回状态 %s", source, response.Status)
	}
	if err := os.MkdirAll(downloadsDir, 0750); err != nil {
		return nil, fmt.Errorf("创建下载目录 %s: %w", downloadsDir, err)
	}
	var buffer bytes.Buffer
	readBuffer := make([]byte, 32*1024)
	total := response.ContentLength
	if total < 0 {
		total = 0
	}
	var downloaded int64
	var lastProgress int64
	for {
		n, readErr := response.Body.Read(readBuffer)
		if n > 0 {
			if _, err := buffer.Write(readBuffer[:n]); err != nil {
				return nil, fmt.Errorf("缓存下载内容 %s: %w", source, err)
			}
			downloaded += int64(n)
			if shouldEmitDownloadProgress(downloaded, lastProgress, total) {
				emitDownloadProgress(progress, source, downloaded, total)
				lastProgress = downloaded
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("读取下载内容 %s: %w", source, readErr)
		}
	}
	if downloaded > 0 && lastProgress != downloaded {
		emitDownloadProgress(progress, source, downloaded, total)
	}
	data := buffer.Bytes()
	cachePath := filepath.Join(downloadsDir, safeCacheName(source))
	if err := os.WriteFile(cachePath, data, 0640); err != nil {
		return nil, fmt.Errorf("写入下载缓存 %s: %w", cachePath, err)
	}
	return data, nil
}

func shouldEmitDownloadProgress(downloaded int64, lastProgress int64, total int64) bool {
	if downloaded <= 0 {
		return false
	}
	if total > 0 && downloaded >= total {
		return true
	}
	return downloaded-lastProgress >= downloadProgressStepBytes
}

func emitDownloadProgress(progress Progress, source string, downloaded int64, total int64) {
	if total > 0 {
		percent := int(float64(downloaded) * 100 / float64(total))
		if percent > 100 {
			percent = 100
		}
		emitProgress(progress, "download: progress %s %s / %s (%d%%)", safeSourceSummary(source), formatBytes(downloaded), formatBytes(total), percent)
		return
	}
	emitProgress(progress, "download: progress %s %s", safeSourceSummary(source), formatBytes(downloaded))
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KiB", "MiB", "GiB"} {
		value = value / unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TiB", value/unit)
}

func safeCacheName(source string) string {
	hash := sha256.Sum256([]byte(source))
	return hex.EncodeToString(hash[:]) + ".download"
}

func verifySHA256(data []byte, shaText string) error {
	shaText = strings.ToLower(strings.TrimSpace(shaText))
	if len(shaText) != sha256.Size*2 {
		return fmt.Errorf("sha256 长度无效")
	}
	if _, err := hex.DecodeString(shaText); err != nil {
		return fmt.Errorf("sha256 必须是 hex: %w", err)
	}
	if got := hashBytes(data); got != shaText {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, shaText)
	}
	return nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isRemoteSource(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

func defaultSources() map[string]Source {
	sources := map[string]Source{
		ResourceRules: {
			Files: []SourceFile{
				{
					URL:    "https://github.com/SagerNet/sing-geosite/releases/download/20260627134952/geosite.db",
					SHA256: "daee9810811ab285d4a32e33602ea77b6a6cd7db2f47086452b66b3f9759884f",
					Path:   "geosite.db",
				},
				{
					URL:    "https://github.com/SagerNet/sing-geoip/releases/download/20260612/geoip.db",
					SHA256: "71484cf35bb48453e26bcc3373a0988a2536588f8e3ca96cda59ff742af6c392",
					Path:   "geoip.db",
				},
			},
		},
	}
	if source, ok := defaultSingBoxSource(); ok {
		sources[ResourceSingBox] = source
		sources[builtinKey(ResourceSingBox, "1.13.14")] = source
	}
	return sources
}

func defaultSingBoxSource() (Source, bool) {
	const version = "1.13.14"
	name := ""
	shaText := ""
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		name = "sing-box-1.13.14-linux-amd64.tar.gz"
		shaText = "f48703461a15476951ac4967cdad339d986f4b8096b4eb3ff0829a500502d697"
	case "linux/arm64":
		name = "sing-box-1.13.14-linux-arm64.tar.gz"
		shaText = "4742df6a4314e8ecc41736849fca6d73b8f9e91b6e8b06ee794ff17ba180579e"
	case "darwin/amd64":
		name = "sing-box-1.13.14-darwin-amd64.tar.gz"
		shaText = "5245d645e847f90bb708da74bc020ae078c28489690756419685c04f56b4e3bb"
	case "darwin/arm64":
		name = "sing-box-1.13.14-darwin-arm64.tar.gz"
		shaText = "73e8967b0fc08e17bce4263ca56ebc394822401a16497a1c4e02316c888202ab"
	default:
		return Source{}, false
	}
	return Source{
		URL:    "https://github.com/SagerNet/sing-box/releases/download/v" + version + "/" + name,
		SHA256: shaText,
	}, true
}

func installSingBox(binDir string, payload []byte, payloadSHA string, sourceSHA string, archiveMember string) error {
	if err := os.MkdirAll(binDir, 0750); err != nil {
		return fmt.Errorf("创建 bin 目录 %s: %w", binDir, err)
	}
	target := filepath.Join(binDir, "sing-box")
	if err := rejectUnmanagedSymlink(target, filepath.Join(binDir, singBoxManagedMarker)); err != nil {
		return err
	}
	binaryTemp, err := writeTempFile(binDir, ".sing-box.tmp-*", payload, 0755)
	if err != nil {
		return err
	}
	markerPath := filepath.Join(binDir, singBoxManagedMarker)
	markerTemp, err := writeTempFile(binDir, ".sing-box.managed.tmp-*", singBoxMarkerData(payloadSHA, sourceSHA, archiveMember), 0640)
	if err != nil {
		_ = os.Remove(binaryTemp)
		return err
	}
	if err := replacePair(target, binaryTemp, markerPath, markerTemp); err != nil {
		return err
	}
	return nil
}

func singBoxMarkerData(payloadSHA string, sourceSHA string, archiveMember string) []byte {
	var builder strings.Builder
	fmt.Fprintf(&builder, "payload %s\n", strings.ToLower(strings.TrimSpace(payloadSHA)))
	if strings.TrimSpace(sourceSHA) != "" {
		fmt.Fprintf(&builder, "source %s\n", strings.ToLower(strings.TrimSpace(sourceSHA)))
	}
	if strings.TrimSpace(archiveMember) != "" {
		fmt.Fprintf(&builder, "archive_member %s\n", strings.TrimSpace(archiveMember))
	}
	return []byte(builder.String())
}

func singBoxSourceInstalled(binDir string, sourceSHA string, archiveMember string) (bool, error) {
	target := filepath.Join(binDir, "sing-box")
	if exists, err := pathExists(target); err != nil || !exists {
		return false, err
	}
	fields, err := readManagedMarkerFields(filepath.Join(binDir, singBoxManagedMarker))
	if err != nil {
		return false, err
	}
	if fields["source"] != strings.ToLower(strings.TrimSpace(sourceSHA)) {
		return false, nil
	}
	if strings.TrimSpace(archiveMember) != "" && fields["archive_member"] != strings.TrimSpace(archiveMember) {
		return false, nil
	}
	return true, nil
}

func rulesFilesInstalled(rulesDir string, files []SourceFile) (bool, error) {
	fields, err := readManagedMarkerFields(filepath.Join(rulesDir, rulesManagedMarker))
	if err != nil {
		return false, err
	}
	if len(fields) == 0 {
		return false, nil
	}
	for _, file := range files {
		if strings.TrimSpace(file.SHA256) == "" {
			return false, nil
		}
		if fields[file.Path] != strings.ToLower(strings.TrimSpace(file.SHA256)) {
			return false, nil
		}
		if exists, err := pathExists(filepath.Join(rulesDir, filepath.Clean(file.Path))); err != nil || !exists {
			return false, err
		}
	}
	return true, nil
}

func readManagedMarkerFields(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取管理标记 %s: %w", path, err)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		value := parts[1]
		if parts[0] != "archive_member" {
			value = strings.ToLower(value)
		}
		fields[parts[0]] = value
	}
	return fields, nil
}

func writeTempFile(dir string, pattern string, data []byte, mode os.FileMode) (string, error) {
	tempFile, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("创建临时文件 %s: %w", dir, err)
	}
	tempPath := tempFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tempFile.Close()
		}
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err = tempFile.Chmod(mode); err != nil {
		return "", fmt.Errorf("设置临时文件权限 %s: %w", tempPath, err)
	}
	if _, err = tempFile.Write(data); err != nil {
		return "", fmt.Errorf("写入临时文件 %s: %w", tempPath, err)
	}
	if err = tempFile.Close(); err != nil {
		closed = true
		return "", fmt.Errorf("关闭临时文件 %s: %w", tempPath, err)
	}
	closed = true
	return tempPath, nil
}

func replacePair(target string, targetTemp string, marker string, markerTemp string) error {
	targetBackup := target + ".previous"
	markerBackup := marker + ".previous"
	_ = os.Remove(targetBackup)
	_ = os.Remove(markerBackup)
	targetExisted, err := renameIfExists(target, targetBackup)
	if err != nil {
		_ = os.Remove(targetTemp)
		_ = os.Remove(markerTemp)
		return err
	}
	markerExisted, err := renameIfExists(marker, markerBackup)
	if err != nil {
		restoreFile(targetBackup, target, targetExisted)
		_ = os.Remove(targetTemp)
		_ = os.Remove(markerTemp)
		return err
	}
	if err := os.Rename(targetTemp, target); err != nil {
		restoreFile(targetBackup, target, targetExisted)
		restoreFile(markerBackup, marker, markerExisted)
		_ = os.Remove(markerTemp)
		return fmt.Errorf("替换 sing-box 二进制 %s: %w", target, err)
	}
	if err := os.Rename(markerTemp, marker); err != nil {
		_ = os.Remove(target)
		restoreFile(targetBackup, target, targetExisted)
		restoreFile(markerBackup, marker, markerExisted)
		return fmt.Errorf("写入 sing-box 管理标记 %s: %w", marker, err)
	}
	_ = os.Remove(targetBackup)
	_ = os.Remove(markerBackup)
	return nil
}

func renameIfExists(path string, backup string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return false, fmt.Errorf("备份路径 %s: %w", path, err)
		}
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("检查路径 %s: %w", path, err)
	}
	return false, nil
}

func restoreFile(backup string, path string, existed bool) {
	if existed {
		_ = os.Rename(backup, path)
		return
	}
	_ = os.Remove(path)
}

func installRules(rulesDir string, archiveData []byte) error {
	parent := filepath.Dir(rulesDir)
	if err := os.MkdirAll(parent, 0750); err != nil {
		return fmt.Errorf("创建 rules 父目录 %s: %w", parent, err)
	}
	if err := rejectUnmanagedSymlink(rulesDir, filepath.Join(rulesDir, rulesManagedMarker)); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(parent, ".rules.tmp-*")
	if err != nil {
		return fmt.Errorf("创建 rules 临时目录 %s: %w", parent, err)
	}
	defer os.RemoveAll(tempDir)
	if err := extractArchiveToDir(archiveData, tempDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, rulesManagedMarker), []byte(hashBytes(archiveData)+"\n"), 0640); err != nil {
		return fmt.Errorf("写入 rules 管理标记 %s: %w", filepath.Join(tempDir, rulesManagedMarker), err)
	}
	return replaceRulesDir(rulesDir, tempDir)
}

func (i *Installer) installRulesFiles(ctx context.Context, downloadsDir string, rulesDir string, files []SourceFile, operation string, progress Progress) error {
	parent := filepath.Dir(rulesDir)
	if err := os.MkdirAll(parent, 0750); err != nil {
		return fmt.Errorf("创建 rules 父目录 %s: %w", parent, err)
	}
	if err := rejectUnmanagedSymlink(rulesDir, filepath.Join(rulesDir, rulesManagedMarker)); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(parent, ".rules.tmp-*")
	if err != nil {
		return fmt.Errorf("创建 rules 临时目录 %s: %w", parent, err)
	}
	defer os.RemoveAll(tempDir)
	markerLines := make([]string, 0, len(files))
	for _, file := range files {
		if err := validateArchiveName(file.Path); err != nil {
			return err
		}
		emitProgress(progress, "%s: prepare rules file %s", operation, file.Path)
		data, err := i.fetch(ctx, downloadsDir, file.URL, file.SHA256, progress)
		if err != nil {
			return err
		}
		target := filepath.Join(tempDir, filepath.Clean(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return fmt.Errorf("创建 rules 文件目录 %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			return fmt.Errorf("写入 rules 文件 %s: %w", target, err)
		}
		emitProgress(progress, "%s: staged rules file %s", operation, file.Path)
		markerLines = append(markerLines, file.Path+" "+hashBytes(data))
	}
	if err := os.WriteFile(filepath.Join(tempDir, rulesManagedMarker), []byte(strings.Join(markerLines, "\n")+"\n"), 0640); err != nil {
		return fmt.Errorf("写入 rules 管理标记 %s: %w", filepath.Join(tempDir, rulesManagedMarker), err)
	}
	return replaceRulesDir(rulesDir, tempDir)
}

func replaceRulesDir(rulesDir string, tempDir string) error {
	backupDir := rulesDir + ".previous"
	_ = os.RemoveAll(backupDir)
	hadExisting := false
	if _, err := os.Lstat(rulesDir); err == nil {
		hadExisting = true
		if err := os.Rename(rulesDir, backupDir); err != nil {
			return fmt.Errorf("备份 rules 目录 %s: %w", rulesDir, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查 rules 目录 %s: %w", rulesDir, err)
	}
	if err := os.Rename(tempDir, rulesDir); err != nil {
		if hadExisting {
			_ = os.Rename(backupDir, rulesDir)
		}
		return fmt.Errorf("替换 rules 目录 %s: %w", rulesDir, err)
	}
	if hadExisting {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func rejectUnmanagedSymlink(target string, markerPath string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("检查目标路径 %s: %w", target, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	if _, err := os.Stat(markerPath); err == nil {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查管理标记 %s: %w", markerPath, err)
	}
	return fmt.Errorf("拒绝覆盖非受管 symlink %s", target)
}

func (i *Installer) uninstallOne(global domain.GlobalConfig, options Options) error {
	emitProgress(options.Progress, "%s: remove %s", options.Operation, options.Resource)
	switch options.Resource {
	case ResourceSingBox:
		if err := uninstallSingBox(global.Paths.Bin); err != nil {
			return err
		}
	case ResourceRules:
		if err := uninstallRules(global.Paths.Rules); err != nil {
			return err
		}
	default:
		return fmt.Errorf("不支持的资源 %q", options.Resource)
	}
	emitProgress(options.Progress, "%s: complete %s", options.Operation, options.Resource)
	return nil
}

func uninstallSingBox(binDir string) error {
	target := filepath.Join(binDir, "sing-box")
	marker := filepath.Join(binDir, singBoxManagedMarker)
	if err := rejectUnmanagedSymlink(target, marker); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 sing-box 二进制 %s: %w", target, err)
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 sing-box 管理标记 %s: %w", marker, err)
	}
	return nil
}

func uninstallRules(rulesDir string) error {
	exists, err := pathExists(rulesDir)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	marker := filepath.Join(rulesDir, rulesManagedMarker)
	if err := rejectUnmanagedSymlink(rulesDir, marker); err != nil {
		return err
	}
	if err := os.RemoveAll(rulesDir); err != nil {
		return fmt.Errorf("删除 rules 目录 %s: %w", rulesDir, err)
	}
	return nil
}

func extractSingBox(data []byte, member string) ([]byte, error) {
	if payload, ok, err := extractSingBoxFromZip(data, member); ok || err != nil {
		return payload, err
	}
	if payload, ok, err := extractSingBoxFromTarGz(data, member); ok || err != nil {
		return payload, err
	}
	if strings.TrimSpace(member) != "" {
		return nil, fmt.Errorf("source 不是可识别归档，不能使用 --archive-member")
	}
	return data, nil
}

func extractSingBoxFromZip(data []byte, member string) ([]byte, bool, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, false, nil
	}
	var selected *zip.File
	for _, file := range reader.File {
		if err := validateArchiveName(file.Name); err != nil {
			return nil, true, err
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if selected == nil && matchesArchiveMember(file.Name, member) {
			selected = file
		}
	}
	if selected == nil {
		return nil, true, fmt.Errorf("归档中找不到 sing-box 成员 %q", member)
	}
	handle, err := selected.Open()
	if err != nil {
		return nil, true, fmt.Errorf("打开归档成员 %s: %w", selected.Name, err)
	}
	defer handle.Close()
	payload, err := io.ReadAll(handle)
	if err != nil {
		return nil, true, fmt.Errorf("读取归档成员 %s: %w", selected.Name, err)
	}
	return payload, true, nil
}

func extractSingBoxFromTarGz(data []byte, member string) ([]byte, bool, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, false, nil
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	var selected []byte
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, true, fmt.Errorf("读取 tar.gz: %w", err)
		}
		if err := validateArchiveName(header.Name); err != nil {
			return nil, true, err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		if selected == nil && matchesArchiveMember(header.Name, member) {
			payload, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, true, fmt.Errorf("读取归档成员 %s: %w", header.Name, err)
			}
			selected = payload
		}
	}
	if selected != nil {
		return selected, true, nil
	}
	return nil, true, fmt.Errorf("归档中找不到 sing-box 成员 %q", member)
}

func extractArchiveToDir(data []byte, targetDir string) error {
	if ok, err := extractZipToDir(data, targetDir); ok || err != nil {
		return err
	}
	if ok, err := extractTarGzToDir(data, targetDir); ok || err != nil {
		return err
	}
	return fmt.Errorf("rules source 必须是 zip 或 tar.gz 归档")
}

func extractZipToDir(data []byte, targetDir string) (bool, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, nil
	}
	for _, file := range reader.File {
		if err := validateArchiveName(file.Name); err != nil {
			return true, err
		}
		if file.FileInfo().IsDir() {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return true, fmt.Errorf("打开归档成员 %s: %w", file.Name, err)
		}
		if err := writeArchiveFile(targetDir, file.Name, handle, file.Mode()); err != nil {
			_ = handle.Close()
			return true, err
		}
		_ = handle.Close()
	}
	return true, nil
}

func extractTarGzToDir(data []byte, targetDir string) (bool, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return false, nil
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return true, fmt.Errorf("读取 tar.gz: %w", err)
		}
		if err := validateArchiveName(header.Name); err != nil {
			return true, err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		if err := writeArchiveFile(targetDir, header.Name, tarReader, header.FileInfo().Mode()); err != nil {
			return true, err
		}
	}
	return true, nil
}

func writeArchiveFile(targetDir string, name string, reader io.Reader, mode os.FileMode) error {
	path := filepath.Join(targetDir, filepath.Clean(name))
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("创建归档成员目录 %s: %w", filepath.Dir(path), err)
	}
	if mode == 0 {
		mode = 0644
	}
	if runtime.GOOS == "windows" {
		mode = 0644
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return fmt.Errorf("创建归档成员文件 %s: %w", path, err)
	}
	defer file.Close()
	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("写入归档成员文件 %s: %w", path, err)
	}
	return nil
}

func validateArchiveName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("归档成员路径不能为空")
	}
	if strings.Contains(name, "\\") {
		return fmt.Errorf("归档成员路径不安全: %s", name)
	}
	if strings.HasPrefix(name, "/") || filepath.VolumeName(name) != "" {
		return fmt.Errorf("归档成员路径不安全: %s", name)
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == ".." {
			return fmt.Errorf("归档成员路径不安全: %s", name)
		}
	}
	cleaned := filepath.Clean(name)
	if cleaned == "." || filepath.IsAbs(cleaned) {
		return fmt.Errorf("归档成员路径不安全: %s", name)
	}
	return nil
}

func matchesArchiveMember(name string, member string) bool {
	if strings.TrimSpace(member) != "" {
		return filepath.Clean(name) == filepath.Clean(member)
	}
	return filepath.Base(name) == "sing-box"
}
