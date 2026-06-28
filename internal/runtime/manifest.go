package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// LoadManifest 严格读取 runtime manifest，文件不存在时返回 nil。
func LoadManifest(path string, generatedDir string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("RuntimeManifest %s: %w", path, err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("RuntimeManifest %s: %w", path, err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != nil {
		if err != io.EOF {
			return nil, fmt.Errorf("RuntimeManifest %s: %w", path, err)
		}
	} else {
		return nil, fmt.Errorf("RuntimeManifest %s: JSON 只允许单个顶层值", path)
	}
	if err := ValidateManifest(manifest, generatedDir); err != nil {
		return nil, fmt.Errorf("RuntimeManifest %s: %w", path, err)
	}
	return &manifest, nil
}

// ValidateManifest 校验 runtime manifest schema、排序和路径安全。
func ValidateManifest(manifest Manifest, generatedDir string) error {
	if manifest.ManifestSchema != ManifestSchema {
		return fmt.Errorf("manifest_schema 必须为 %q", ManifestSchema)
	}
	if manifest.ManifestVersion != ManifestVersion {
		return fmt.Errorf("manifest_version 必须为 %d", ManifestVersion)
	}
	if strings.TrimSpace(manifest.ConfigSHA256) == "" {
		return fmt.Errorf("config_sha256 不能为空")
	}
	if manifest.InstanceSHA256 == nil {
		return fmt.Errorf("instance_sha256 不能为空")
	}
	if strings.TrimSpace(manifest.GeneratedAt) == "" {
		return fmt.Errorf("generated_at 不能为空")
	}
	for index, file := range manifest.Files {
		if err := validateManifestFile(file, generatedDir); err != nil {
			return fmt.Errorf("files[%d]: %w", index, err)
		}
		if index > 0 && manifest.Files[index-1].RelativePath > file.RelativePath {
			return fmt.Errorf("files 必须按 relative_path 稳定排序")
		}
	}
	return nil
}

// SortManifestFiles 按 relative_path 稳定排序 manifest files。
func SortManifestFiles(files []ManifestFile) {
	sort.SliceStable(files, func(i int, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
}

// validateManifestFile 校验单个 manifest file 的路径约束。
func validateManifestFile(file ManifestFile, generatedDir string) error {
	if strings.TrimSpace(file.Instance) == "" {
		return fmt.Errorf("instance 不能为空")
	}
	if strings.TrimSpace(file.Path) == "" {
		return fmt.Errorf("path 不能为空")
	}
	if strings.TrimSpace(file.RelativePath) == "" {
		return fmt.Errorf("relative_path 不能为空")
	}
	if strings.TrimSpace(file.SHA256) == "" {
		return fmt.Errorf("sha256 不能为空")
	}
	if strings.TrimSpace(file.Service) == "" {
		return fmt.Errorf("service 不能为空")
	}
	if err := validateRelativePath(file.RelativePath); err != nil {
		return err
	}
	if !isPathUnder(generatedDir, file.Path) {
		return fmt.Errorf("path 必须位于 paths.generated 下")
	}
	return nil
}

// validateRelativePath 校验 manifest relative_path 只使用安全 slash 相对路径。
func validateRelativePath(relativePath string) error {
	if strings.Contains(relativePath, "\\") {
		return fmt.Errorf("relative_path 不允许反斜杠")
	}
	if path.IsAbs(relativePath) {
		return fmt.Errorf("relative_path 不允许绝对路径")
	}
	cleaned := path.Clean(relativePath)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return fmt.Errorf("relative_path 不允许路径穿越")
	}
	if cleaned != relativePath {
		return fmt.Errorf("relative_path 必须是规范路径")
	}
	return nil
}

// isPathUnder 判断 target 是否位于 parent 目录下。
func isPathUnder(parent string, target string) bool {
	parent = filepath.Clean(parent)
	target = filepath.Clean(target)
	relative, err := filepath.Rel(parent, target)
	if err != nil {
		return false
	}
	return relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
