package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// NormalizeGlobalPaths 将全局配置中的路径清理为规范绝对路径。
func NormalizeGlobalPaths(baseDir string, config *domain.GlobalConfig) error {
	if config == nil {
		return fmt.Errorf("GlobalConfig cannot be nil")
	}

	resolvedBase, err := normalizeBaseDir(baseDir)
	if err != nil {
		return err
	}

	paths := &config.Paths
	if paths.Bin, err = normalizePathField(resolvedBase, paths.Bin); err != nil {
		return fmt.Errorf("paths.bin: %w", err)
	}
	if paths.Rules, err = normalizePathField(resolvedBase, paths.Rules); err != nil {
		return fmt.Errorf("paths.rules: %w", err)
	}
	if paths.Instances, err = normalizePathField(resolvedBase, paths.Instances); err != nil {
		return fmt.Errorf("paths.instances: %w", err)
	}
	if paths.Runtime, err = normalizePathField(resolvedBase, paths.Runtime); err != nil {
		return fmt.Errorf("paths.runtime: %w", err)
	}
	if paths.Generated, err = normalizePathField(resolvedBase, paths.Generated); err != nil {
		return fmt.Errorf("paths.generated: %w", err)
	}
	if paths.Publish, err = normalizePathField(resolvedBase, paths.Publish); err != nil {
		return fmt.Errorf("paths.publish: %w", err)
	}
	if paths.Traffic, err = normalizePathField(resolvedBase, paths.Traffic); err != nil {
		return fmt.Errorf("paths.traffic: %w", err)
	}
	if paths.Downloads, err = normalizePathField(resolvedBase, paths.Downloads); err != nil {
		return fmt.Errorf("paths.downloads: %w", err)
	}
	if paths.Logs, err = normalizePathField(resolvedBase, paths.Logs); err != nil {
		return fmt.Errorf("paths.logs: %w", err)
	}
	return nil
}

// NormalizeSubConfigPaths 将订阅服务配置中的路径清理为规范绝对路径。
func NormalizeSubConfigPaths(baseDir string, config *domain.SubConfig) error {
	if config == nil {
		return fmt.Errorf("SubConfig cannot be nil")
	}

	resolvedBase, err := normalizeBaseDir(baseDir)
	if err != nil {
		return err
	}
	config.TemplatesDir, err = normalizePathField(resolvedBase, config.TemplatesDir)
	if err != nil {
		return fmt.Errorf("templates_dir: %w", err)
	}
	return nil
}

// normalizeBaseDir 清理并转为绝对 base dir。
func normalizeBaseDir(baseDir string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", fmt.Errorf("base dir cannot be empty")
	}
	if hasTraversalSegment(baseDir) {
		return "", fmt.Errorf("base dir must not contain path traversal")
	}
	absolute, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("clean base dir: %w", err)
	}
	return filepath.Clean(absolute), nil
}

// normalizePathField 清理单个路径字段。
func normalizePathField(baseDir string, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("cannot be empty")
	}
	if hasTraversalSegment(value) {
		return "", fmt.Errorf("path traversal is not allowed")
	}
	cleaned := filepath.Clean(value)
	if filepath.IsAbs(cleaned) {
		return cleaned, nil
	}

	resolved := filepath.Clean(filepath.Join(baseDir, cleaned))
	if !isPathUnder(baseDir, resolved) && resolved != baseDir {
		return "", fmt.Errorf("relative path must be under base dir")
	}
	return resolved, nil
}

// hasTraversalSegment 判断路径中是否包含穿越片段或反斜杠。
func hasTraversalSegment(value string) bool {
	if strings.Contains(value, "\\") {
		return true
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// isPathUnder 判断 target 是否位于 parent 下。
func isPathUnder(parent string, target string) bool {
	relative, err := filepath.Rel(parent, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
