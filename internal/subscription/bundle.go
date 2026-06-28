package subscription

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

const bundleManifestName = "manifest.json"

// BundleSummary 描述一次订阅 bundle 导出的摘要。
type BundleSummary struct {
	Source string   `json:"source"`
	File   string   `json:"file"`
	Nodes  int      `json:"nodes"`
	Users  []string `json:"users"`
}

// ExportResult 表示构建订阅 bundle 的结果。
type ExportResult struct {
	Data      []byte
	Manifest  domain.BundleManifest
	Summaries []BundleSummary
}

// ImportResult 表示导入订阅 bundle 的结果。
type ImportResult struct {
	Inputs  int
	Nodes   int
	Replace bool
}

// BuildBundle 将订阅 input 编码为安全 zip bundle。
func BuildBundle(inputs []domain.SubscriptionInput, generatedAt time.Time) (*ExportResult, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("没有可导出的订阅 input")
	}
	if _, err := BuildIndexFromInputs(inputs); err != nil {
		return nil, err
	}

	files := make(map[string][]byte, len(inputs))
	for _, input := range inputs {
		name := input.Source + ".json"
		if err := domain.ValidateSubscriptionInputFilename(name); err != nil {
			return nil, fmt.Errorf("订阅 input 文件名 %s: %w", name, err)
		}
		if _, exists := files[name]; exists {
			return nil, fmt.Errorf("订阅 input 文件名重复: %s", name)
		}
		data, err := MarshalStable(input)
		if err != nil {
			return nil, fmt.Errorf("编码订阅 input %s: %w", input.Source, err)
		}
		files[name] = data
	}

	manifest := domain.BundleManifest{
		BundleSchema:    "sbox.sub-bundle",
		BundleVersion:   1,
		Source:          "all",
		GeneratedAt:     generatedAt.Format(time.RFC3339),
		InputsSHA256:    make(map[string]string, len(files)),
		TemplateVersion: BuiltinTemplateVersion,
		Access: domain.AccessConfig{
			Type: "none",
		},
	}
	names := sortedFileNames(files)
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		manifest.InputsSHA256[name] = hex.EncodeToString(sum[:])
	}
	if err := domain.ValidateBundleManifest(manifest); err != nil {
		return nil, err
	}

	data, err := encodeBundleZip(manifest, files)
	if err != nil {
		return nil, err
	}
	return &ExportResult{
		Data:      data,
		Manifest:  manifest,
		Summaries: SummariesForInputs(inputs),
	}, nil
}

// ImportBundle 校验 bundle 后导入到 base dir 的 inputs 目录。
func ImportBundle(baseDir string, bundlePath string, replaceAll bool) (*ImportResult, error) {
	files, inputs, err := readBundle(bundlePath)
	if err != nil {
		return nil, err
	}

	finalInputs := inputs
	finalFiles := files
	if !replaceAll {
		existingFiles, existingInputs, err := loadExistingInputsForMerge(InputsDir(baseDir), files)
		if err != nil {
			return nil, err
		}
		finalInputs = append(existingInputs, inputs...)
		finalFiles = make(map[string][]byte, len(existingFiles)+len(files))
		for name, data := range existingFiles {
			finalFiles[name] = data
		}
		for name, data := range files {
			finalFiles[name] = data
		}
	}
	index, err := BuildIndexFromInputs(finalInputs)
	if err != nil {
		return nil, err
	}

	if err := replaceInputDir(baseDir, finalFiles); err != nil {
		return nil, err
	}
	return &ImportResult{
		Inputs:  len(files),
		Nodes:   len(index.Nodes),
		Replace: replaceAll,
	}, nil
}

// SummariesForInputs 生成订阅 input 摘要，不包含任何敏感字段。
func SummariesForInputs(inputs []domain.SubscriptionInput) []BundleSummary {
	summaries := make([]BundleSummary, 0, len(inputs))
	for _, input := range inputs {
		users := make(map[string]struct{})
		for _, node := range input.Nodes {
			if node.User != "" {
				users[node.User] = struct{}{}
			}
		}
		userNames := make([]string, 0, len(users))
		for user := range users {
			userNames = append(userNames, user)
		}
		sort.Strings(userNames)
		summaries = append(summaries, BundleSummary{
			Source: input.Source,
			File:   input.Source + ".json",
			Nodes:  len(input.Nodes),
			Users:  userNames,
		})
	}
	return summaries
}

// WriteFileAtomic 将数据写入临时文件后原子替换目标文件。
func WriteFileAtomic(target string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return fmt.Errorf("设置临时文件权限: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("替换文件 %s: %w", target, err)
	}
	return nil
}

// encodeBundleZip 按固定成员顺序编码 bundle zip。
func encodeBundleZip(manifest domain.BundleManifest, files map[string][]byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	manifestData, err := MarshalStable(manifest)
	if err != nil {
		return nil, fmt.Errorf("编码 manifest: %w", err)
	}
	if err := writeZipMember(writer, bundleManifestName, manifestData); err != nil {
		return nil, err
	}
	for _, name := range sortedFileNames(files) {
		if err := writeZipMember(writer, "inputs/"+name, files[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("关闭 bundle zip: %w", err)
	}
	return buffer.Bytes(), nil
}

// writeZipMember 写入一个普通 zip 文件成员。
func writeZipMember(writer *zip.Writer, name string, data []byte) error {
	member, err := writer.Create(name)
	if err != nil {
		return fmt.Errorf("创建 zip 成员 %s: %w", name, err)
	}
	if _, err := member.Write(data); err != nil {
		return fmt.Errorf("写入 zip 成员 %s: %w", name, err)
	}
	return nil
}

// readBundle 完整校验 zip、manifest、hash、路径和 input schema。
func readBundle(bundlePath string) (map[string][]byte, []domain.SubscriptionInput, error) {
	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开订阅 bundle %s: %w", bundlePath, err)
	}
	defer reader.Close()

	var manifestData []byte
	files := make(map[string][]byte)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			return nil, nil, fmt.Errorf("bundle 不允许目录成员: %s", file.Name)
		}
		kind, name, err := validateBundleMember(file.Name)
		if err != nil {
			return nil, nil, err
		}
		data, err := readZipFile(file)
		if err != nil {
			return nil, nil, err
		}
		if kind == bundleManifestName {
			if manifestData != nil {
				return nil, nil, fmt.Errorf("bundle manifest 重复")
			}
			manifestData = data
			continue
		}
		if _, exists := files[name]; exists {
			return nil, nil, fmt.Errorf("bundle input 重复: %s", name)
		}
		files[name] = data
	}
	if manifestData == nil {
		return nil, nil, fmt.Errorf("bundle 缺少 manifest.json")
	}

	var manifest domain.BundleManifest
	if err := config.DecodeStrict(manifestData, "json", "BundleManifest", &manifest); err != nil {
		return nil, nil, fmt.Errorf("解析 bundle manifest: %w", err)
	}
	if err := domain.ValidateBundleManifest(manifest); err != nil {
		return nil, nil, err
	}
	if err := validateBundleHashes(manifest, files); err != nil {
		return nil, nil, err
	}

	inputs := make([]domain.SubscriptionInput, 0, len(files))
	for _, name := range sortedFileNames(files) {
		input, err := DecodeInput(name, files[name])
		if err != nil {
			return nil, nil, fmt.Errorf("bundle input %s: %w", name, err)
		}
		inputs = append(inputs, input)
	}
	if _, err := BuildIndexFromInputs(inputs); err != nil {
		return nil, nil, err
	}
	return files, inputs, nil
}

// validateBundleMember 校验 zip 成员路径并返回成员类型和 input basename。
func validateBundleMember(name string) (string, string, error) {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) || path.Clean(name) != name {
		return "", "", fmt.Errorf("bundle 成员路径不安全: %s", name)
	}
	if name == bundleManifestName {
		return bundleManifestName, "", nil
	}
	if !strings.HasPrefix(name, "inputs/") {
		return "", "", fmt.Errorf("bundle 包含未知成员: %s", name)
	}
	base := strings.TrimPrefix(name, "inputs/")
	if base == "" || strings.Contains(base, "/") {
		return "", "", fmt.Errorf("bundle input 路径不安全: %s", name)
	}
	if err := domain.ValidateSubscriptionInputFilename(base); err != nil {
		return "", "", fmt.Errorf("bundle input 文件名 %s: %w", base, err)
	}
	return "input", base, nil
}

// readZipFile 读取单个 zip 成员内容。
func readZipFile(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("打开 zip 成员 %s: %w", file.Name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取 zip 成员 %s: %w", file.Name, err)
	}
	return data, nil
}

// validateBundleHashes 校验 manifest 中的文件集合和 sha256。
func validateBundleHashes(manifest domain.BundleManifest, files map[string][]byte) error {
	if len(manifest.InputsSHA256) != len(files) {
		return fmt.Errorf("bundle manifest inputs_sha256 与 input 文件集合不一致")
	}
	for name, data := range files {
		want, exists := manifest.InputsSHA256[name]
		if !exists {
			return fmt.Errorf("bundle manifest 缺少 input hash: %s", name)
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(want, hex.EncodeToString(sum[:])) {
			return fmt.Errorf("bundle input hash mismatch: %s", name)
		}
	}
	for name := range manifest.InputsSHA256 {
		if _, exists := files[name]; !exists {
			return fmt.Errorf("bundle manifest 包含不存在的 input: %s", name)
		}
	}
	return nil
}

// loadExistingInputsForMerge 加载非 replace-all 导入时需要保留的既有 input 和原始文件。
func loadExistingInputsForMerge(dir string, incoming map[string][]byte) (map[string][]byte, []domain.SubscriptionInput, error) {
	files, err := LoadInputsFromDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	keptFiles := make(map[string][]byte, len(files))
	inputs := make([]domain.SubscriptionInput, 0, len(files))
	for _, file := range files {
		if _, replaced := incoming[file.Name]; replaced {
			continue
		}
		keptFiles[file.Name] = file.Data
		inputs = append(inputs, file.Input)
	}
	return keptFiles, inputs, nil
}

// replaceInputDir 用已校验的文件集合原子替换 inputs 目录。
func replaceInputDir(baseDir string, files map[string][]byte) error {
	if err := os.MkdirAll(baseDir, 0750); err != nil {
		return fmt.Errorf("创建 base dir %s: %w", baseDir, err)
	}
	tempDir, err := os.MkdirTemp(baseDir, ".inputs.tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时 input 目录: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := writeFilesToDir(tempDir, files); err != nil {
		return err
	}

	inputsDir := InputsDir(baseDir)
	backupDir := filepath.Join(baseDir, ".inputs.backup-"+time.Now().Format("20060102150405.000000000"))
	hadExisting := false
	if _, err := os.Stat(inputsDir); err == nil {
		if err := os.Rename(inputsDir, backupDir); err != nil {
			return fmt.Errorf("备份旧 input 目录: %w", err)
		}
		hadExisting = true
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查旧 input 目录: %w", err)
	}

	if err := os.Rename(tempDir, inputsDir); err != nil {
		if hadExisting {
			_ = os.Rename(backupDir, inputsDir)
		}
		return fmt.Errorf("替换 input 目录: %w", err)
	}
	if hadExisting {
		if err := os.RemoveAll(backupDir); err != nil {
			return fmt.Errorf("清理旧 input 目录: %w", err)
		}
	}
	return nil
}

// writeFilesToDir 将文件集合写入指定目录。
func writeFilesToDir(dir string, files map[string][]byte) error {
	for _, name := range sortedFileNames(files) {
		if err := domain.ValidateSubscriptionInputFilename(name); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, name), files[name], 0640); err != nil {
			return fmt.Errorf("写入 input %s: %w", name, err)
		}
	}
	return nil
}

// sortedFileNames 返回 map key 的稳定排序结果。
func sortedFileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
