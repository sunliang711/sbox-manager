package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// AgentConfigSet 表示已加载并通过组合校验的 agent 配置集合。
type AgentConfigSet struct {
	BaseDir   string              `json:"base_dir"`
	Global    domain.GlobalConfig `json:"global"`
	Instances []domain.Instance   `json:"instances"`
}

// LoadAgentConfigSet 从 base dir 只读加载全局配置和全部 instance 配置。
func LoadAgentConfigSet(baseDir string) (*AgentConfigSet, error) {
	resolvedBase, err := normalizeBaseDir(baseDir)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(resolvedBase, "config.yaml")
	global, err := LoadGlobalConfig(configPath, resolvedBase)
	if err != nil {
		return nil, err
	}

	instances, err := LoadInstances(global.Paths.Instances, *global)
	if err != nil {
		return nil, err
	}
	if err := domain.ValidateConfigSet(*global, instances); err != nil {
		return nil, fmt.Errorf("ConfigSet %s: %w", baseDir, err)
	}
	return &AgentConfigSet{
		BaseDir:   resolvedBase,
		Global:    *global,
		Instances: instances,
	}, nil
}

// LoadInstances 从目录只读加载全部 instance 配置文件。
func LoadInstances(dir string, global domain.GlobalConfig) ([]domain.Instance, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("Instances %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if isInstanceConfigFile(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	instances := make([]domain.Instance, 0, len(names))
	for _, name := range names {
		instance, err := LoadInstance(filepath.Join(dir, name), global)
		if err != nil {
			return nil, err
		}
		instances = append(instances, *instance)
	}
	return instances, nil
}

// FindInstance 按名称查找配置集合中的 instance。
func (s *AgentConfigSet) FindInstance(name string) (domain.Instance, bool) {
	if s == nil {
		return domain.Instance{}, false
	}
	for _, instance := range s.Instances {
		if instance.Name == name {
			return instance, true
		}
	}
	return domain.Instance{}, false
}

// TargetInstances 返回指定 target 对应的 instance 集合。
func (s *AgentConfigSet) TargetInstances(target string) ([]domain.Instance, error) {
	if s == nil {
		return nil, fmt.Errorf("配置集合不能为空")
	}
	if strings.TrimSpace(target) == "" {
		targets := make([]domain.Instance, 0, len(s.Instances))
		for _, instance := range s.Instances {
			if instance.Enabled {
				targets = append(targets, instance)
			}
		}
		return targets, nil
	}
	instance, ok := s.FindInstance(target)
	if !ok {
		return nil, fmt.Errorf("instance %q 不存在", target)
	}
	return []domain.Instance{instance}, nil
}

// isInstanceConfigFile 判断文件名是否是受支持的 instance 配置文件。
func isInstanceConfigFile(name string) bool {
	if isDraftConfigFile(name) {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

// isDraftConfigFile 判断文件名是否为编辑器产生的草稿或临时配置。
func isDraftConfigFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, ".draft") || strings.Contains(lower, ".tmp") || strings.Contains(lower, ".swp")
}
