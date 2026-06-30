package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
)

const (
	// ManifestName 是 agent 配置备份中的固定 manifest 文件名。
	ManifestName = "backup_manifest.json"
)

// ExportResult 表示一次 agent 配置备份导出的结果。
type ExportResult struct {
	Data     []byte
	Manifest domain.BackupManifest
	Files    []string
}

// ImportResult 表示一次 agent 配置备份导入的结果。
type ImportResult struct {
	Files []string
	Force bool
}

type importFile struct {
	Target    string
	Data      []byte
	Mode      os.FileMode
	temp      string
	backup    string
	existed   bool
	installed bool
}

// Build 从 agent base dir 构建只包含配置文件的新格式备份 zip。
func Build(baseDir string, generatedAt time.Time) (*ExportResult, error) {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		return nil, err
	}
	files, err := collectFiles(set)
	if err != nil {
		return nil, err
	}
	manifest := domain.BackupManifest{
		BackupSchema:  "sbox.agent-backup",
		BackupVersion: 1,
		GeneratedAt:   generatedAt.Format(time.RFC3339),
		FilesSHA256:   make(map[string]string, len(files)),
	}
	for _, name := range sortedNames(files) {
		sum := sha256.Sum256(files[name])
		manifest.FilesSHA256[name] = hex.EncodeToString(sum[:])
	}
	if err := domain.ValidateBackupManifest(manifest); err != nil {
		return nil, err
	}
	data, err := encodeZip(manifest, files)
	if err != nil {
		return nil, err
	}
	return &ExportResult{Data: data, Manifest: manifest, Files: sortedNames(files)}, nil
}

// Import 完整校验备份包后写入 agent 配置文件。
func Import(baseDir string, backupPath string, force bool) (*ImportResult, error) {
	files, _, err := Read(backupPath)
	if err != nil {
		return nil, err
	}
	resolvedBase, err := cleanBaseDir(baseDir)
	if err != nil {
		return nil, err
	}
	global, _, err := validateConfigFiles(resolvedBase, files)
	if err != nil {
		return nil, err
	}
	if err := validateImportTarget(resolvedBase, global.Paths.Instances, "paths.instances"); err != nil {
		return nil, err
	}
	if !force {
		if err := ensureNoExistingConfig(resolvedBase, global.Paths.Instances); err != nil {
			return nil, err
		}
	}
	if err := writeImportedFiles(resolvedBase, global.Paths.Instances, files); err != nil {
		return nil, err
	}
	return &ImportResult{Files: sortedNames(files), Force: force}, nil
}

// Read 读取并校验 agent 配置备份 zip，返回已校验的成员文件。
func Read(backupPath string) (map[string][]byte, domain.BackupManifest, error) {
	reader, err := zip.OpenReader(backupPath)
	if err != nil {
		return nil, domain.BackupManifest{}, fmt.Errorf("open agent backup %s: %w", backupPath, err)
	}
	defer reader.Close()

	var manifestData []byte
	files := make(map[string][]byte)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			return nil, domain.BackupManifest{}, fmt.Errorf("agent backup does not allow directory member: %s", file.Name)
		}
		if file.Name == "manifest.json" {
			return nil, domain.BackupManifest{}, fmt.Errorf("subscription bundle or legacy backup is not accepted: %s", file.Name)
		}
		if file.Name == ManifestName {
			if manifestData != nil {
				return nil, domain.BackupManifest{}, fmt.Errorf("duplicate agent backup manifest")
			}
			data, err := readZipFile(file)
			if err != nil {
				return nil, domain.BackupManifest{}, err
			}
			manifestData = data
			continue
		}
		if err := domain.ValidateBackupFileName(file.Name); err != nil {
			return nil, domain.BackupManifest{}, fmt.Errorf("agent backup member %s: %w", file.Name, err)
		}
		if _, exists := files[file.Name]; exists {
			return nil, domain.BackupManifest{}, fmt.Errorf("duplicate agent backup member: %s", file.Name)
		}
		data, err := readZipFile(file)
		if err != nil {
			return nil, domain.BackupManifest{}, err
		}
		files[file.Name] = data
	}
	if manifestData == nil {
		return nil, domain.BackupManifest{}, fmt.Errorf("agent backup missing %s", ManifestName)
	}

	var manifest domain.BackupManifest
	if err := config.DecodeStrict(manifestData, "json", "BackupManifest", &manifest); err != nil {
		return nil, domain.BackupManifest{}, fmt.Errorf("parse agent backup manifest: %w", err)
	}
	if err := domain.ValidateBackupManifest(manifest); err != nil {
		return nil, domain.BackupManifest{}, err
	}
	if err := validateHashes(manifest, files); err != nil {
		return nil, domain.BackupManifest{}, err
	}
	return files, manifest, nil
}

// WriteFileAtomic 将数据写入临时文件后原子替换目标。
func WriteFileAtomic(target string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return fmt.Errorf("set temp file permissions: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("replace file %s: %w", target, err)
	}
	return nil
}

// collectFiles 收集 agent 配置备份允许进入 zip 的文件。
func collectFiles(set *config.AgentConfigSet) (map[string][]byte, error) {
	files := make(map[string][]byte)
	configData, err := os.ReadFile(filepath.Join(set.BaseDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read agent config.yaml: %w", err)
	}
	files["config.yaml"] = configData

	entries, err := os.ReadDir(set.Global.Paths.Instances)
	if err != nil {
		return nil, fmt.Errorf("read instances directory %s: %w", set.Global.Paths.Instances, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !isInstanceConfigName(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("read instance file info %s: %w", entry.Name(), err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		relativeName := "instances/" + entry.Name()
		if err := domain.ValidateBackupFileName(relativeName); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(set.Global.Paths.Instances, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read instance config %s: %w", entry.Name(), err)
		}
		files[relativeName] = data
	}
	return files, nil
}

// encodeZip 按固定顺序写入 agent backup zip。
func encodeZip(manifest domain.BackupManifest, files map[string][]byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	manifestData, err := marshalStable(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode backup manifest: %w", err)
	}
	if err := writeZipMember(writer, ManifestName, manifestData); err != nil {
		return nil, err
	}
	for _, name := range sortedNames(files) {
		if err := writeZipMember(writer, name, files[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close agent backup zip: %w", err)
	}
	return buffer.Bytes(), nil
}

// writeZipMember 写入一个普通 zip 文件成员。
func writeZipMember(writer *zip.Writer, name string, data []byte) error {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) || path.Clean(name) != name {
		return fmt.Errorf("zip member path is unsafe: %s", name)
	}
	member, err := writer.Create(name)
	if err != nil {
		return fmt.Errorf("create zip member %s: %w", name, err)
	}
	if _, err := member.Write(data); err != nil {
		return fmt.Errorf("write zip member %s: %w", name, err)
	}
	return nil
}

// readZipFile 读取单个 zip 成员内容。
func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("open zip member %s: %w", file.Name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read zip member %s: %w", file.Name, err)
	}
	return data, nil
}

// validateHashes 校验 manifest 文件集合和 sha256 与 zip 成员完全一致。
func validateHashes(manifest domain.BackupManifest, files map[string][]byte) error {
	if len(manifest.FilesSHA256) != len(files) {
		return fmt.Errorf("agent backup manifest files_sha256 does not match file set")
	}
	for name, data := range files {
		want, exists := manifest.FilesSHA256[name]
		if !exists {
			return fmt.Errorf("agent backup manifest missing file hash: %s", name)
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(want, hex.EncodeToString(sum[:])) {
			return fmt.Errorf("agent backup hash mismatch: %s", name)
		}
	}
	for name := range manifest.FilesSHA256 {
		if _, exists := files[name]; !exists {
			return fmt.Errorf("agent backup manifest contains missing file: %s", name)
		}
	}
	return nil
}

// validateConfigFiles 对备份中的 config 和 instance 配置执行严格 schema 校验。
func validateConfigFiles(baseDir string, files map[string][]byte) (domain.GlobalConfig, []domain.Instance, error) {
	configData, ok := files["config.yaml"]
	if !ok {
		return domain.GlobalConfig{}, nil, fmt.Errorf("agent backup missing config.yaml")
	}
	tempDir, err := os.MkdirTemp("", "sbox-agent-backup-validate-*")
	if err != nil {
		return domain.GlobalConfig{}, nil, fmt.Errorf("create validation temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, configData, 0600); err != nil {
		return domain.GlobalConfig{}, nil, fmt.Errorf("write validation config: %w", err)
	}
	global, err := config.LoadGlobalConfig(configPath, baseDir)
	if err != nil {
		return domain.GlobalConfig{}, nil, err
	}

	instances := make([]domain.Instance, 0)
	for _, name := range sortedNames(files) {
		if !strings.HasPrefix(name, "instances/") {
			continue
		}
		tempPath := filepath.Join(tempDir, filepath.Base(name))
		if err := os.WriteFile(tempPath, files[name], 0600); err != nil {
			return domain.GlobalConfig{}, nil, fmt.Errorf("write validation instance %s: %w", name, err)
		}
		instance, err := config.LoadInstance(tempPath, *global)
		if err != nil {
			return domain.GlobalConfig{}, nil, err
		}
		instances = append(instances, *instance)
	}
	if err := domain.ValidateConfigSet(*global, instances); err != nil {
		return domain.GlobalConfig{}, nil, err
	}
	return *global, instances, nil
}

// ensureNoExistingConfig 在未传 --force 时拒绝覆盖已有 agent 配置。
func ensureNoExistingConfig(baseDir string, instancesDir string) error {
	if exists, err := pathExists(filepath.Join(baseDir, "config.yaml")); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("target config.yaml already exists; use --force to overwrite")
	}
	names, err := existingInstanceConfigNames(instancesDir)
	if err != nil {
		return err
	}
	if len(names) > 0 {
		return fmt.Errorf("target instance config already exists; use --force to overwrite")
	}
	return nil
}

// writeImportedFiles 将已校验的备份成员按文件集合事务写入目标配置路径。
func writeImportedFiles(baseDir string, instancesDir string, files map[string][]byte) error {
	importFiles, err := buildImportFiles(baseDir, instancesDir, files)
	if err != nil {
		return err
	}
	return replaceImportFiles(importFiles)
}

// buildImportFiles 构造导入目标文件清单，config.yaml 最后替换以降低中断影响。
func buildImportFiles(baseDir string, instancesDir string, files map[string][]byte) ([]importFile, error) {
	result := make([]importFile, 0, len(files))
	for _, name := range sortedNames(files) {
		if !strings.HasPrefix(name, "instances/") {
			continue
		}
		target := filepath.Join(instancesDir, filepath.Base(name))
		if err := validateImportTarget(baseDir, target, name); err != nil {
			return nil, err
		}
		result = append(result, importFile{Target: target, Data: files[name], Mode: 0640})
	}
	configTarget := filepath.Join(baseDir, "config.yaml")
	if err := validateImportTarget(baseDir, configTarget, "config.yaml"); err != nil {
		return nil, err
	}
	result = append(result, importFile{Target: configTarget, Data: files["config.yaml"], Mode: 0640})
	return result, nil
}

// replaceImportFiles 先写完所有临时文件，再替换目标；失败时回滚已替换文件。
func replaceImportFiles(files []importFile) error {
	for index := range files {
		if err := prepareImportTemp(&files[index]); err != nil {
			cleanupImportTemps(files)
			return err
		}
	}
	for index := range files {
		if err := replaceOneImportFile(&files[index]); err != nil {
			rollbackErr := rollbackImportFiles(files)
			if rollbackErr != nil {
				return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
			}
			return err
		}
	}
	cleanupImportBackups(files)
	return nil
}

// prepareImportTemp 在目标同目录预写临时文件，确保替换前所有内容可落盘。
func prepareImportTemp(file *importFile) error {
	dir := filepath.Dir(file.Target)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(file.Target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	file.temp = temp.Name()
	if _, err := temp.Write(file.Data); err != nil {
		temp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := temp.Chmod(file.Mode); err != nil {
		temp.Close()
		return fmt.Errorf("set temp file permissions: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return nil
}

// replaceOneImportFile 替换单个目标文件，并保留原文件用于失败回滚。
func replaceOneImportFile(file *importFile) error {
	exists, err := pathExists(file.Target)
	if err != nil {
		return err
	}
	file.existed = exists
	if exists {
		backup, err := reserveImportBackupName(file.Target)
		if err != nil {
			return err
		}
		file.backup = backup
		if err := os.Rename(file.Target, file.backup); err != nil {
			return fmt.Errorf("backup target file %s: %w", file.Target, err)
		}
	}
	if err := os.Rename(file.temp, file.Target); err != nil {
		return fmt.Errorf("replace file %s: %w", file.Target, err)
	}
	file.temp = ""
	file.installed = true
	return nil
}

// reserveImportBackupName 预留一个不存在的同目录备份路径。
func reserveImportBackupName(target string) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".bak-*")
	if err != nil {
		return "", fmt.Errorf("create backup placeholder file: %w", err)
	}
	name := temp.Name()
	if err := temp.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close backup placeholder file: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return "", fmt.Errorf("remove backup placeholder file: %w", err)
	}
	return name, nil
}

// rollbackImportFiles 尽量恢复已替换的目标文件。
func rollbackImportFiles(files []importFile) error {
	var errs []string
	for index := len(files) - 1; index >= 0; index-- {
		file := files[index]
		if file.temp != "" {
			if err := os.Remove(file.temp); err != nil && !os.IsNotExist(err) {
				errs = append(errs, err.Error())
			}
		}
		if file.installed {
			if err := os.Remove(file.Target); err != nil && !os.IsNotExist(err) {
				errs = append(errs, err.Error())
			}
		}
		if file.backup != "" {
			if err := os.Rename(file.backup, file.Target); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// cleanupImportTemps 清理尚未替换的临时文件。
func cleanupImportTemps(files []importFile) {
	for _, file := range files {
		if file.temp != "" {
			os.Remove(file.temp)
		}
	}
}

// cleanupImportBackups 清理成功替换后保留的旧文件备份。
func cleanupImportBackups(files []importFile) {
	for _, file := range files {
		if file.backup != "" {
			os.Remove(file.backup)
		}
	}
}

// validateImportTarget 校验导入写入目标必须位于 base dir 内部。
func validateImportTarget(baseDir string, target string, field string) error {
	relative, err := filepath.Rel(baseDir, filepath.Clean(target))
	if err != nil {
		return fmt.Errorf("%s path is unsafe: %w", field, err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s must be inside base dir: %s", field, target)
	}
	return nil
}

// existingInstanceConfigNames 返回目标 instances 目录下已有的配置文件名。
func existingInstanceConfigNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read target instances directory %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && isInstanceConfigName(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// isInstanceConfigName 判断文件名是否是受支持的 instance 配置文件。
func isInstanceConfigName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

// pathExists 返回路径是否存在。
func pathExists(target string) (bool, error) {
	_, err := os.Stat(target)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// cleanBaseDir 解析并校验导入目标 base dir。
func cleanBaseDir(baseDir string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", fmt.Errorf("base dir cannot be empty")
	}
	if strings.Contains(baseDir, "\\") {
		return "", fmt.Errorf("base dir must not contain backslashes")
	}
	for _, segment := range strings.Split(strings.ReplaceAll(baseDir, "\\", "/"), "/") {
		if segment == ".." {
			return "", fmt.Errorf("base dir must not contain path traversal")
		}
	}
	absolute, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("clean base dir: %w", err)
	}
	return filepath.Clean(absolute), nil
}

// marshalStable 使用稳定 JSON 格式编码 manifest。
func marshalStable(value interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// sortedNames 返回 map key 的稳定排序结果。
func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
