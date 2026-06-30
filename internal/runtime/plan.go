package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/sunliang711/sbox-manager/internal/domain"
	"github.com/sunliang711/sbox-manager/internal/generator/singbox"
)

// BuildPlan 根据配置和目标 instance 构造 runtime diff 计划，过程只读。
func BuildPlan(global domain.GlobalConfig, instances []domain.Instance, target string) (*Plan, error) {
	targetInstances, targetScope, fullScope, err := selectTargetInstances(instances, target)
	if err != nil {
		return nil, err
	}

	configHash, err := hashStable(global)
	if err != nil {
		return nil, fmt.Errorf("calculate global config hash: %w", err)
	}
	plan := &Plan{
		ManifestPath:   filepath.Join(global.Paths.Runtime, "manifest.json"),
		RuntimeDir:     global.Paths.Runtime,
		GeneratedDir:   global.Paths.Generated,
		Target:         target,
		ConfigSHA256:   configHash,
		InstanceSHA256: map[string]string{},
		targetScope:    targetScope,
		fullScope:      fullScope,
	}

	for _, instance := range targetInstances {
		instanceHash, err := hashStable(instance)
		if err != nil {
			return nil, fmt.Errorf("calculate instance %s hash: %w", instance.Name, err)
		}
		plan.InstanceSHA256[instance.Name] = instanceHash

		data, err := singbox.GenerateWithInstances(global, instances, instance)
		if err != nil {
			return nil, err
		}
		filePath := filepath.Join(global.Paths.Generated, "sing-box", instance.Name+".json")
		relativePath, err := relativeRuntimePath(global.Paths.Runtime, filePath)
		if err != nil {
			return nil, err
		}
		plan.DesiredFiles = append(plan.DesiredFiles, DesiredFile{
			ManifestFile: ManifestFile{
				Instance:     instance.Name,
				Path:         filePath,
				RelativePath: relativePath,
				SHA256:       hashBytes(data),
				Service:      serviceName(instance.Name),
			},
			Data: data,
		})
	}
	sort.SliceStable(plan.DesiredFiles, func(i int, j int) bool {
		return plan.DesiredFiles[i].RelativePath < plan.DesiredFiles[j].RelativePath
	})

	existing, err := LoadManifest(plan.ManifestPath, plan.GeneratedDir)
	if err != nil {
		return nil, err
	}
	plan.Existing = existing
	if err := buildChanges(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// HasChanges 判断 plan 是否包含 create/update/delete 动作。
func (p *Plan) HasChanges() bool {
	if p == nil {
		return false
	}
	for _, change := range p.Changes {
		if change.Action != ActionNoChange {
			return true
		}
	}
	return false
}

// Services 返回当前目标生成物对应的服务名，顺序稳定且去重。
func (p *Plan) Services() []string {
	if p == nil {
		return nil
	}
	seen := map[string]struct{}{}
	services := make([]string, 0, len(p.DesiredFiles))
	for _, file := range p.DesiredFiles {
		if _, exists := seen[file.Service]; exists {
			continue
		}
		seen[file.Service] = struct{}{}
		services = append(services, file.Service)
	}
	sort.Strings(services)
	return services
}

// selectTargetInstances 按 CLI target 规则选择参与生成的 instance。
func selectTargetInstances(instances []domain.Instance, target string) ([]domain.Instance, map[string]struct{}, bool, error) {
	if target == "" {
		selected := make([]domain.Instance, 0, len(instances))
		for _, instance := range instances {
			if instance.Enabled {
				selected = append(selected, instance)
			}
		}
		return selected, map[string]struct{}{}, true, nil
	}
	for _, instance := range instances {
		if instance.Name == target {
			return []domain.Instance{instance}, map[string]struct{}{target: {}}, false, nil
		}
	}
	return nil, nil, false, fmt.Errorf("instance %q does not exist", target)
}

// buildChanges 对比期望文件、manifest 和磁盘文件，生成稳定 diff。
func buildChanges(plan *Plan) error {
	existingByRelative := map[string]ManifestFile{}
	if plan.Existing != nil {
		for _, file := range plan.Existing.Files {
			existingByRelative[file.RelativePath] = file
		}
	}

	desiredByRelative := map[string]DesiredFile{}
	for _, desired := range plan.DesiredFiles {
		desiredByRelative[desired.RelativePath] = desired
		action, err := desiredAction(plan, desired, existingByRelative[desired.RelativePath])
		if err != nil {
			return err
		}
		plan.Changes = append(plan.Changes, Change{
			Action:       action,
			Instance:     desired.Instance,
			Path:         desired.Path,
			RelativePath: desired.RelativePath,
			SHA256:       desired.SHA256,
			Service:      desired.Service,
			Data:         desired.Data,
		})
	}

	if plan.Existing != nil {
		for _, existing := range plan.Existing.Files {
			if !plan.inTargetScope(existing.Instance) {
				continue
			}
			if _, desired := desiredByRelative[existing.RelativePath]; desired {
				continue
			}
			plan.Changes = append(plan.Changes, Change{
				Action:       ActionDelete,
				Instance:     existing.Instance,
				Path:         existing.Path,
				RelativePath: existing.RelativePath,
				SHA256:       existing.SHA256,
				Service:      existing.Service,
			})
		}
	}

	sort.SliceStable(plan.Changes, func(i int, j int) bool {
		if plan.Changes[i].RelativePath == plan.Changes[j].RelativePath {
			return plan.Changes[i].Action < plan.Changes[j].Action
		}
		return plan.Changes[i].RelativePath < plan.Changes[j].RelativePath
	})
	return nil
}

// desiredAction 判断单个期望文件相对现状的动作。
func desiredAction(plan *Plan, desired DesiredFile, existing ManifestFile) (Action, error) {
	if existing.RelativePath == "" {
		return ActionCreate, nil
	}
	diskHash, exists, err := fileSHA256(existing.Path)
	if err != nil {
		return "", err
	}
	if !exists {
		return ActionCreate, nil
	}
	if diskHash != desired.SHA256 || existing.SHA256 != desired.SHA256 {
		return ActionUpdate, nil
	}
	if plan.Existing == nil {
		return ActionUpdate, nil
	}
	if plan.Existing.ConfigSHA256 != plan.ConfigSHA256 {
		return ActionUpdate, nil
	}
	if plan.Existing.InstanceSHA256[desired.Instance] != plan.InstanceSHA256[desired.Instance] {
		return ActionUpdate, nil
	}
	return ActionNoChange, nil
}

// inTargetScope 判断 manifest 中的文件是否属于本次目标范围。
func (p *Plan) inTargetScope(instance string) bool {
	if p.fullScope {
		return true
	}
	_, ok := p.targetScope[instance]
	return ok
}

// relativeRuntimePath 返回以 runtime 目录为基准的 slash 相对路径。
func relativeRuntimePath(runtimeDir string, filePath string) (string, error) {
	relative, err := filepath.Rel(runtimeDir, filePath)
	if err != nil {
		return "", fmt.Errorf("calculate relative_path: %w", err)
	}
	relative = filepath.ToSlash(relative)
	if err := validateRelativePath(relative); err != nil {
		return "", err
	}
	return relative, nil
}

// serviceName 返回 sing-box instance 的 systemd 服务名契约。
func serviceName(instance string) string {
	return "sbox@" + instance + ".service"
}

// hashStable 对结构化对象做稳定 JSON 后计算 SHA-256。
func hashStable(value interface{}) (string, error) {
	data, err := singbox.MarshalStable(value)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

// hashBytes 计算字节切片的 SHA-256 hex。
func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fileSHA256 读取文件并计算 SHA-256，文件不存在时返回 exists=false。
func fileSHA256(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read generated file %s: %w", path, err)
	}
	return hashBytes(data), true, nil
}
