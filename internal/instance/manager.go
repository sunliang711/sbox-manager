package instance

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
)

var defaultEditorCommands = []string{"vim", "vi", "nvim", "nano"}

// InitOptions 描述 agent 环境初始化参数。
type InitOptions struct {
	ExternalHost  string
	Force         bool
	AllowExisting bool
}

// AddOptions 描述新增 instance 参数。
type AddOptions struct {
	Name              string
	Template          string
	FromFile          string
	AllocatePorts     bool
	KeepTemplatePorts bool
}

// CloneOptions 描述克隆 instance 参数。
type CloneOptions struct {
	Source        string
	Target        string
	AllocatePorts bool
}

// Init 创建标准目录和默认 config.yaml。
func Init(baseDir string, options InitOptions) error {
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("base dir 不能为空")
	}
	configPath := filepath.Join(baseDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil && !options.Force {
		if options.AllowExisting {
			global, err := config.LoadGlobalConfig(configPath, baseDir)
			if err != nil {
				return err
			}
			return EnsureDirectories(*global)
		}
		return fmt.Errorf("配置文件 %s 已存在，覆盖请使用 --force", configPath)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查配置文件 %s: %w", configPath, err)
	}

	global := domain.DefaultGlobalConfig()
	global.ExternalHost = strings.TrimSpace(options.ExternalHost)
	normalized := global
	if err := config.NormalizeGlobalPaths(baseDir, &normalized); err != nil {
		return err
	}
	if err := EnsureDirectories(normalized); err != nil {
		return err
	}
	data, err := yaml.Marshal(global)
	if err != nil {
		return fmt.Errorf("编码默认配置: %w", err)
	}
	if err := writeFileAtomic(configPath, data, 0640); err != nil {
		return fmt.Errorf("写入配置文件 %s: %w", configPath, err)
	}
	return nil
}

// EnsureDirectories 创建 agent 受管目录。
func EnsureDirectories(global domain.GlobalConfig) error {
	for _, dir := range []string{
		global.Paths.Bin,
		global.Paths.Rules,
		global.Paths.Instances,
		global.Paths.Runtime,
		global.Paths.Generated,
		global.Paths.Publish,
		global.Paths.Traffic,
		global.Paths.Downloads,
		global.Paths.Logs,
	} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("创建目录 %s: %w", dir, err)
		}
	}
	return nil
}

// Add 新增 instance 配置文件。
func Add(baseDir string, options AddOptions) (domain.Instance, error) {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		return domain.Instance{}, err
	}
	if _, exists := set.FindInstance(options.Name); exists {
		return domain.Instance{}, fmt.Errorf("instance %q 已存在", options.Name)
	}
	instance, err := buildInstance(set.Global, set.Instances, options)
	if err != nil {
		return domain.Instance{}, err
	}
	if err := WriteInstance(set.Global, set.Instances, instance); err != nil {
		return domain.Instance{}, err
	}
	return instance, nil
}

func buildInstance(global domain.GlobalConfig, existing []domain.Instance, options AddOptions) (domain.Instance, error) {
	var instance domain.Instance
	if options.FromFile != "" {
		loaded, err := loadInstanceDraft(options.FromFile, global)
		if err != nil {
			return domain.Instance{}, err
		}
		instance = loaded
		instance.Name = options.Name
	} else {
		created, err := templateInstance(global, options.Name, options.Template)
		if err != nil {
			return domain.Instance{}, err
		}
		instance = created
	}
	if options.AllocatePorts && !options.KeepTemplatePorts {
		if err := allocatePorts(global, existing, &instance); err != nil {
			return domain.Instance{}, err
		}
	}
	domain.ApplyInstanceDefaults(&instance)
	if err := domain.ValidateInstance(global, &instance); err != nil {
		return domain.Instance{}, err
	}
	return instance, nil
}

func templateInstance(global domain.GlobalConfig, name string, template string) (domain.Instance, error) {
	instance := domain.DefaultInstance(global)
	instance.Name = name
	instance.API.Enabled = false
	instance.Role = template
	if template == "" {
		instance.Role = "edge"
	}
	instance.Outbounds = []domain.Outbound{{Name: "direct", Type: "direct"}}
	switch instance.Role {
	case "edge":
		instance.Inbounds = []domain.Inbound{vmessInbound("vmess-main", 24000)}
		instance.Inbounds[0].Subscription.Remark = instance.Name
		instance.Route = domain.RouteConfig{Default: "direct"}
	case "relay":
		instance.Inbounds = []domain.Inbound{shadowsocksInbound("ss-main", 24000)}
		instance.Route = domain.RouteConfig{Default: "direct"}
	case "urltest":
		instance.Inbounds = []domain.Inbound{vmessInbound("vmess-main", 24000)}
		instance.Inbounds[0].Subscription.Remark = instance.Name
		instance.Groups = []domain.Group{domain.DefaultGroup("auto", "urltest")}
		instance.Groups[0].Outbounds = []string{"direct"}
		instance.Route = domain.RouteConfig{Default: "auto"}
	default:
		return domain.Instance{}, fmt.Errorf("不支持的 template %q", template)
	}
	domain.ApplyInstanceDefaults(&instance)
	return instance, nil
}

func vmessInbound(name string, port int) domain.Inbound {
	inbound := domain.DefaultInbound(name, "vmess")
	inbound.Port = port
	inbound.Users = []domain.InboundUser{
		{
			Name: "alice",
			UUID: mustUUID(),
		},
	}
	inbound.Subscription.Enabled = true
	inbound.Subscription.User = "alice"
	inbound.Subscription.Remark = name
	return inbound
}

func shadowsocksInbound(name string, port int) domain.Inbound {
	inbound := domain.DefaultInbound(name, "shadowsocks")
	inbound.Port = port
	inbound.Method = "2022-blake3-aes-256-gcm"
	inbound.Users = []domain.InboundUser{
		{
			Name:     "alice",
			Password: mustRandomHex(32),
			Method:   inbound.Method,
		},
	}
	return inbound
}

func allocatePorts(global domain.GlobalConfig, existing []domain.Instance, target *domain.Instance) error {
	used := usedPorts(existing)
	for index := range target.Inbounds {
		port, err := config.FirstAvailablePort(global.PortRanges.Inbound, used)
		if err != nil {
			return err
		}
		target.Inbounds[index].Port = port
		used[port] = struct{}{}
	}
	return nil
}

func usedPorts(instances []domain.Instance) map[int]struct{} {
	used := map[int]struct{}{}
	for _, instance := range instances {
		for _, inbound := range instance.Inbounds {
			if inbound.Port > 0 {
				used[inbound.Port] = struct{}{}
			}
		}
	}
	return used
}

// Clone 克隆 instance 配置并可重新分配 inbound 端口。
func Clone(baseDir string, options CloneOptions) (domain.Instance, error) {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		return domain.Instance{}, err
	}
	source, ok := set.FindInstance(options.Source)
	if !ok {
		return domain.Instance{}, fmt.Errorf("instance %q 不存在", options.Source)
	}
	if _, exists := set.FindInstance(options.Target); exists {
		return domain.Instance{}, fmt.Errorf("instance %q 已存在", options.Target)
	}
	cloned := cloneInstanceValue(source)
	cloned.Name = options.Target
	updateClonedSubscriptionRemarks(&cloned, options.Source, options.Target)
	if options.AllocatePorts {
		if err := allocatePorts(set.Global, set.Instances, &cloned); err != nil {
			return domain.Instance{}, err
		}
	}
	if err := WriteInstance(set.Global, set.Instances, cloned); err != nil {
		return domain.Instance{}, err
	}
	return cloned, nil
}

// updateClonedSubscriptionRemarks 将默认实例名 remark 更新为克隆目标名，保留用户自定义 remark。
func updateClonedSubscriptionRemarks(instance *domain.Instance, sourceName string, targetName string) {
	if instance == nil {
		return
	}
	for index := range instance.Inbounds {
		if !instance.Inbounds[index].Subscription.Enabled {
			continue
		}
		if instance.Inbounds[index].Subscription.Remark == sourceName {
			instance.Inbounds[index].Subscription.Remark = targetName
		}
	}
}

// cloneInstanceValue 复制 instance 中所有可变切片，避免克隆修改反向污染源实例。
func cloneInstanceValue(source domain.Instance) domain.Instance {
	cloned := source
	cloned.Labels = append([]string(nil), source.Labels...)
	cloned.Inbounds = append([]domain.Inbound(nil), source.Inbounds...)
	for index := range cloned.Inbounds {
		cloned.Inbounds[index].Users = append([]domain.InboundUser(nil), source.Inbounds[index].Users...)
	}
	cloned.Outbounds = append([]domain.Outbound(nil), source.Outbounds...)
	cloned.Groups = append([]domain.Group(nil), source.Groups...)
	for index := range cloned.Groups {
		cloned.Groups[index].Outbounds = append([]string(nil), source.Groups[index].Outbounds...)
	}
	cloned.Route.Rules = append([]domain.RouteRule(nil), source.Route.Rules...)
	for index := range cloned.Route.Rules {
		cloned.Route.Rules[index].Values = append([]string(nil), source.Route.Rules[index].Values...)
	}
	cloned.Traffic.Scopes = append([]string(nil), source.Traffic.Scopes...)
	return cloned
}

// WriteInstance 校验并写入 instance YAML。
func WriteInstance(global domain.GlobalConfig, existing []domain.Instance, instance domain.Instance) error {
	if err := domain.ValidateInstance(global, &instance); err != nil {
		return err
	}
	next := make([]domain.Instance, 0, len(existing)+1)
	for _, item := range existing {
		if item.Name != instance.Name {
			next = append(next, item)
		}
	}
	next = append(next, instance)
	if err := domain.ValidateConfigSet(global, next); err != nil {
		return err
	}
	if err := os.MkdirAll(global.Paths.Instances, 0750); err != nil {
		return fmt.Errorf("创建 instances 目录 %s: %w", global.Paths.Instances, err)
	}
	data, err := yaml.Marshal(instance)
	if err != nil {
		return fmt.Errorf("编码 instance %s: %w", instance.Name, err)
	}
	path := filepath.Join(global.Paths.Instances, instance.Name+".yaml")
	if err := writeFileAtomic(path, data, 0640); err != nil {
		return fmt.Errorf("写入 instance 配置 %s: %w", path, err)
	}
	return nil
}

// Remove 删除 instance 配置，并在 purge 时删除对应 generated 文件。
func Remove(baseDir string, name string, purge bool) error {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		return err
	}
	if _, ok := set.FindInstance(name); !ok {
		return fmt.Errorf("instance %q 不存在", name)
	}
	path, err := FindInstancePath(set.Global, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除 instance 配置 %s: %w", path, err)
	}
	if purge {
		generatedPath := filepath.Join(set.Global.Paths.Generated, "sing-box", name+".json")
		if err := os.Remove(generatedPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("删除 generated 文件 %s: %w", generatedPath, err)
		}
	}
	return nil
}

// FindInstancePath 返回已存在的 instance 配置路径。
func FindInstancePath(global domain.GlobalConfig, name string) (string, error) {
	for _, extension := range []string{".yaml", ".yml", ".json"} {
		path := filepath.Join(global.Paths.Instances, name+extension)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("检查 instance 配置 %s: %w", path, err)
		}
	}
	return "", fmt.Errorf("instance 配置 %q 不存在", name)
}

// ListLines 返回稳定排序的 instance 摘要行。
func ListLines(instances []domain.Instance, verbose bool) []string {
	items := append([]domain.Instance(nil), instances...)
	sort.SliceStable(items, func(i int, j int) bool {
		return items[i].Name < items[j].Name
	})
	lines := make([]string, 0, len(items))
	for _, instance := range items {
		status := "disabled"
		if instance.Enabled {
			status = "enabled"
		}
		if verbose {
			lines = append(lines, fmt.Sprintf("%s\t%s\t%s\tinbounds=%d\toutbounds=%d\tgroups=%d", instance.Name, status, instance.Role, len(instance.Inbounds), len(instance.Outbounds), len(instance.Groups)))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", instance.Name, status, instance.Role))
	}
	return lines
}

// MemberList 返回 group 成员列表。
func MemberList(set *config.AgentConfigSet, instanceName string, groupName string) ([]string, error) {
	group, _, err := findGroup(set, instanceName, groupName)
	if err != nil {
		return nil, err
	}
	return append([]string(nil), group.Outbounds...), nil
}

// MemberAdd 添加 selector/urltest group 成员。
func MemberAdd(baseDir string, instanceName string, groupName string, member string) error {
	return updateMember(baseDir, instanceName, groupName, member, true)
}

// MemberRemove 移除 selector/urltest group 成员。
func MemberRemove(baseDir string, instanceName string, groupName string, member string) error {
	return updateMember(baseDir, instanceName, groupName, member, false)
}

func updateMember(baseDir string, instanceName string, groupName string, member string, add bool) error {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		return err
	}
	instanceValue, instanceIndex, err := findInstance(set, instanceName)
	if err != nil {
		return err
	}
	groupIndex := -1
	for index, group := range instanceValue.Groups {
		if group.Name == groupName {
			groupIndex = index
			break
		}
	}
	if groupIndex < 0 {
		return fmt.Errorf("group %q 不存在", groupName)
	}
	group := &instanceValue.Groups[groupIndex]
	if group.Type != "selector" && group.Type != "urltest" {
		return fmt.Errorf("group %q 不是 selector 或 urltest", groupName)
	}
	if add {
		for _, outbound := range group.Outbounds {
			if outbound == member {
				set.Instances[instanceIndex] = instanceValue
				return nil
			}
		}
		group.Outbounds = append(group.Outbounds, member)
	} else {
		next := group.Outbounds[:0]
		for _, outbound := range group.Outbounds {
			if outbound != member {
				next = append(next, outbound)
			}
		}
		group.Outbounds = next
	}
	set.Instances[instanceIndex] = instanceValue
	return WriteInstance(set.Global, set.Instances, instanceValue)
}

func findGroup(set *config.AgentConfigSet, instanceName string, groupName string) (domain.Group, domain.Instance, error) {
	instanceValue, _, err := findInstance(set, instanceName)
	if err != nil {
		return domain.Group{}, domain.Instance{}, err
	}
	for _, group := range instanceValue.Groups {
		if group.Name == groupName {
			if group.Type != "selector" && group.Type != "urltest" {
				return domain.Group{}, domain.Instance{}, fmt.Errorf("group %q 不是 selector 或 urltest", groupName)
			}
			return group, instanceValue, nil
		}
	}
	return domain.Group{}, domain.Instance{}, fmt.Errorf("group %q 不存在", groupName)
}

func findInstance(set *config.AgentConfigSet, name string) (domain.Instance, int, error) {
	if set == nil {
		return domain.Instance{}, -1, fmt.Errorf("配置集合不能为空")
	}
	for index, instance := range set.Instances {
		if instance.Name == name {
			return instance, index, nil
		}
	}
	return domain.Instance{}, -1, fmt.Errorf("instance %q 不存在", name)
}

func loadInstanceDraft(path string, global domain.GlobalConfig) (domain.Instance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("读取 instance 模板 %s: %w", path, err)
	}
	instance := domain.DefaultInstance(global)
	format, err := formatForPath(path)
	if err != nil {
		return domain.Instance{}, err
	}
	if err := config.DecodeStrict(data, format, "Instance", &instance); err != nil {
		return domain.Instance{}, err
	}
	domain.ApplyInstanceDefaults(&instance)
	return instance, nil
}

func formatForPath(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml", nil
	case ".json":
		return "json", nil
	default:
		return "", fmt.Errorf("不支持的配置文件扩展名")
	}
}

// EditFileWithCommand 使用参数数组调用编辑器，调用方负责校验草稿内容。
func EditFileWithCommand(path string, editor string) error {
	editor, err := resolveEditorCommand(editor)
	if err != nil {
		return err
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return fmt.Errorf("editor 不能为空")
	}
	args := append(parts[1:], path)
	command := exec.Command(parts[0], args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("执行 editor %s: %w", parts[0], err)
	}
	return nil
}

// resolveEditorCommand 解析编辑器命令，未指定时按常见系统编辑器顺序自动查找。
func resolveEditorCommand(editor string) (string, error) {
	editor = strings.TrimSpace(editor)
	if editor != "" {
		return editor, nil
	}
	editor = strings.TrimSpace(os.Getenv("EDITOR"))
	if editor != "" {
		return editor, nil
	}
	for _, candidate := range defaultEditorCommands {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("未找到可用 editor，请使用 --editor、设置 EDITOR 或安装 vim/vi/nvim/nano")
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("创建目录 %s: %w", dir, err)
	}
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("创建临时文件 %s: %w", dir, err)
	}
	tempPath := tempFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempPath)
	}()
	if err := tempFile.Chmod(mode); err != nil {
		return fmt.Errorf("设置临时文件权限 %s: %w", tempPath, err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("写入临时文件 %s: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		closed = true
		return fmt.Errorf("关闭临时文件 %s: %w", tempPath, err)
	}
	closed = true
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("替换文件 %s: %w", path, err)
	}
	return nil
}

func mustUUID() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(data)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func mustRandomHex(size int) string {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data)
}
