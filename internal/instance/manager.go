package instance

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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
	Members           []string
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
	data, err := renderGlobalConfigYAML(global)
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
		if len(options.Members) > 0 {
			return domain.Instance{}, fmt.Errorf("--members 只能与内置 urltest 模板一起使用")
		}
		loaded, err := loadInstanceDraft(options.FromFile, global)
		if err != nil {
			return domain.Instance{}, err
		}
		instance = loaded
		instance.Name = options.Name
	} else {
		created, err := templateInstance(global, existing, options.Name, options.Template, options.Members)
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

func templateInstance(global domain.GlobalConfig, existing []domain.Instance, name string, template string, members []string) (domain.Instance, error) {
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
		if len(members) > 0 {
			return domain.Instance{}, fmt.Errorf("--members 只能用于 urltest 模板")
		}
		instance.Inbounds = []domain.Inbound{vmessInbound("vmess-main", 24000)}
		instance.Inbounds[0].Subscription.Remark = instance.Name
		instance.Inbounds = append(instance.Inbounds, localProxyInbounds()...)
		instance.Route = domain.RouteConfig{Default: "direct"}
	case "relay":
		if len(members) > 0 {
			return domain.Instance{}, fmt.Errorf("--members 只能用于 urltest 模板")
		}
		instance.Inbounds = []domain.Inbound{shadowsocksInbound("ss-main", 24000)}
		instance.Inbounds = append(instance.Inbounds, localProxyInbounds()...)
		instance.Route = domain.RouteConfig{Default: "direct"}
	case "urltest":
		if len(members) == 0 {
			return domain.Instance{}, fmt.Errorf("urltest 模板必须通过 --members 指定已有 instance")
		}
		instance.Outbounds = nil
		instance.Inbounds = []domain.Inbound{vmessInbound("vmess-main", 24000)}
		instance.Inbounds[0].Subscription.Remark = instance.Name
		instance.Inbounds = append(instance.Inbounds, localProxyInbounds()...)
		instance.Groups = []domain.Group{domain.DefaultGroup("auto", "urltest")}
		if err := addRefMembersToInstance(&instance, existing, "auto", members); err != nil {
			return domain.Instance{}, err
		}
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

// localProxyInbounds 返回模板默认附带的本地 HTTP/SOCKS 代理入口。
func localProxyInbounds() []domain.Inbound {
	return []domain.Inbound{
		localProxyInbound("local-socks", "socks5", 17000),
		localProxyInbound("local-http", "http", 18000),
	}
}

// localProxyInbound 创建仅监听 loopback 的本地代理入口。
func localProxyInbound(name string, inboundType string, port int) domain.Inbound {
	inbound := domain.DefaultInbound(name, inboundType)
	inbound.Listen = "127.0.0.1"
	inbound.Port = port
	return inbound
}

// addRefMembersToInstance 将已有 instance 作为 ref outbound 成员加入指定 group。
func addRefMembersToInstance(instance *domain.Instance, existing []domain.Instance, groupName string, members []string) error {
	for _, member := range members {
		if err := addRefMemberToInstance(instance, existing, groupName, member); err != nil {
			return err
		}
	}
	return nil
}

// addRefMemberToInstance 为单个 member instance 创建 ref outbound 并加入 group。
func addRefMemberToInstance(instance *domain.Instance, existing []domain.Instance, groupName string, member string) error {
	if instance == nil {
		return fmt.Errorf("instance 不能为空")
	}
	memberInstance, ok := findInstanceByName(existing, member)
	if !ok {
		return fmt.Errorf("member instance %q 不存在", member)
	}
	if memberInstance.Name == instance.Name {
		return fmt.Errorf("member instance 不能引用自身")
	}
	inbound, ok := selectRefMemberInbound(memberInstance)
	if !ok {
		return fmt.Errorf("member instance %q 必须包含 socks5 或 http inbound", member)
	}
	ref := refMemberRef(memberInstance.Name, inbound.Name)
	for _, outbound := range instance.Outbounds {
		if outbound.Type == "ref" && outbound.Ref == ref {
			return addGroupOutbound(instance, groupName, outbound.Name)
		}
	}
	outboundName := allocateRefOutboundName(instance.Outbounds, refOutboundName(memberInstance.Name, inbound.Name), ref)
	instance.Outbounds = append(instance.Outbounds, domain.Outbound{
		Name: outboundName,
		Type: "ref",
		Ref:  ref,
	})
	return addGroupOutbound(instance, groupName, outboundName)
}

// removeRefMemberFromInstance 从 group 和 outbounds 中移除指定 member 的 ref 成员。
func removeRefMemberFromInstance(instance *domain.Instance, existing []domain.Instance, groupName string, member string) error {
	if instance == nil {
		return fmt.Errorf("instance 不能为空")
	}
	groupIndex, err := findMutableGroup(instance, groupName)
	if err != nil {
		return err
	}
	candidates := map[string]struct{}{}
	for _, outbound := range instance.Outbounds {
		if outbound.Type == "ref" {
			memberName, _, ok := parseRefMemberWithInstances(outbound.Ref, existing)
			if !ok {
				memberName, _, ok = parseRefMember(outbound.Ref)
			}
			if ok && memberName == member {
				candidates[outbound.Name] = struct{}{}
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	group := &instance.Groups[groupIndex]
	removed := map[string]struct{}{}
	nextGroupOutbounds := group.Outbounds[:0]
	for _, outboundName := range group.Outbounds {
		if _, ok := candidates[outboundName]; ok {
			removed[outboundName] = struct{}{}
			continue
		}
		nextGroupOutbounds = append(nextGroupOutbounds, outboundName)
	}
	group.Outbounds = nextGroupOutbounds
	if len(removed) == 0 {
		return nil
	}
	referenced := referencedGroupOutbounds(instance.Groups)
	nextOutbounds := instance.Outbounds[:0]
	for _, outbound := range instance.Outbounds {
		if _, removedFromGroup := removed[outbound.Name]; removedFromGroup {
			if _, stillReferenced := referenced[outbound.Name]; !stillReferenced {
				continue
			}
		}
		nextOutbounds = append(nextOutbounds, outbound)
	}
	instance.Outbounds = nextOutbounds
	return nil
}

// addGroupOutbound 将 outbound 名称加入 selector/urltest group，已存在时保持幂等。
func addGroupOutbound(instance *domain.Instance, groupName string, outboundName string) error {
	groupIndex, err := findMutableGroup(instance, groupName)
	if err != nil {
		return err
	}
	group := &instance.Groups[groupIndex]
	for _, current := range group.Outbounds {
		if current == outboundName {
			return nil
		}
	}
	group.Outbounds = append(group.Outbounds, outboundName)
	return nil
}

// referencedGroupOutbounds 收集所有 group 仍在引用的 outbound 名称。
func referencedGroupOutbounds(groups []domain.Group) map[string]struct{} {
	referenced := map[string]struct{}{}
	for _, group := range groups {
		for _, outboundName := range group.Outbounds {
			referenced[outboundName] = struct{}{}
		}
	}
	return referenced
}

// findMutableGroup 返回可维护成员的 group 下标。
func findMutableGroup(instance *domain.Instance, groupName string) (int, error) {
	for index, group := range instance.Groups {
		if group.Name != groupName {
			continue
		}
		if group.Type != "selector" && group.Type != "urltest" {
			return -1, fmt.Errorf("group %q 不是 selector 或 urltest", groupName)
		}
		return index, nil
	}
	return -1, fmt.Errorf("group %q 不存在", groupName)
}

// findInstanceByName 按名称查找 instance。
func findInstanceByName(instances []domain.Instance, name string) (domain.Instance, bool) {
	for _, instance := range instances {
		if instance.Name == name {
			return instance, true
		}
	}
	return domain.Instance{}, false
}

// selectRefMemberInbound 选择 member 中可被 ref outbound 引用的入口，优先 socks5。
func selectRefMemberInbound(instance domain.Instance) (domain.Inbound, bool) {
	for _, inbound := range instance.Inbounds {
		if inbound.Type == "socks5" {
			return inbound, true
		}
	}
	for _, inbound := range instance.Inbounds {
		if inbound.Type == "http" {
			return inbound, true
		}
	}
	return domain.Inbound{}, false
}

// refMemberRef 生成 `<instance>.<inbound>` 格式引用。
func refMemberRef(instanceName string, inboundName string) string {
	return instanceName + "." + inboundName
}

// refOutboundName 生成 member ref outbound 的稳定名称。
func refOutboundName(instanceName string, inboundName string) string {
	return instanceName + "-" + inboundName
}

// allocateRefOutboundName 为 ref outbound 分配稳定名称，发生碰撞时追加短哈希后缀。
func allocateRefOutboundName(outbounds []domain.Outbound, baseName string, ref string) string {
	if outboundNameAvailable(outbounds, baseName) {
		return baseName
	}
	hashedName := fmt.Sprintf("%s-%08x", baseName, refHash(ref))
	if outboundNameAvailable(outbounds, hashedName) {
		return hashedName
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%08x-%d", baseName, refHash(ref), index)
		if outboundNameAvailable(outbounds, candidate) {
			return candidate
		}
	}
}

// outboundNameAvailable 判断 outbound 名称是否未被占用。
func outboundNameAvailable(outbounds []domain.Outbound, name string) bool {
	for _, outbound := range outbounds {
		if outbound.Name == name {
			return false
		}
	}
	return true
}

// refHash 返回 ref 的稳定短哈希。
func refHash(ref string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(ref))
	return hash.Sum32()
}

func allocatePorts(global domain.GlobalConfig, existing []domain.Instance, target *domain.Instance) error {
	used := usedPorts(existing)
	for index := range target.Inbounds {
		port, err := config.FirstAvailablePort(portRangeForInbound(global, target.Inbounds[index]), used)
		if err != nil {
			return err
		}
		target.Inbounds[index].Port = port
		used[port] = struct{}{}
	}
	if target.API.Enabled {
		_, _, err := splitListenAddress(target.API.Listen)
		if err == nil {
			apiPort, err := config.FirstAvailablePort(global.PortRanges.API, used)
			if err != nil {
				return err
			}
			target.API.Listen = replaceListenPort(target.API.Listen, apiPort)
			used[apiPort] = struct{}{}
		}
	}
	return nil
}

// portRangeForInbound 根据 inbound 类型选择对应的自动分配端口段。
func portRangeForInbound(global domain.GlobalConfig, inbound domain.Inbound) domain.PortRange {
	switch inbound.Type {
	case "socks5":
		if !isLoopbackListen(inbound.Listen) {
			return global.PortRanges.Inbound
		}
		return global.PortRanges.LocalSocks
	case "http":
		if !isLoopbackListen(inbound.Listen) {
			return global.PortRanges.Inbound
		}
		return global.PortRanges.LocalHTTP
	default:
		return global.PortRanges.Inbound
	}
}

// isLoopbackListen 判断 inbound listen 是否仅绑定本机地址。
func isLoopbackListen(value string) bool {
	host := strings.TrimSpace(value)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func usedPorts(instances []domain.Instance) map[int]struct{} {
	used := map[int]struct{}{}
	for _, instance := range instances {
		for _, inbound := range instance.Inbounds {
			if inbound.Port > 0 {
				used[inbound.Port] = struct{}{}
			}
		}
		if instance.API.Enabled {
			_, port, err := splitListenAddress(instance.API.Listen)
			if err == nil && port > 0 {
				used[port] = struct{}{}
			}
		}
	}
	return used
}

// splitListenAddress 解析 HOST:PORT 监听地址，用于 clone 时判断 API 端口是否可重分配。
func splitListenAddress(value string) (string, int, error) {
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// replaceListenPort 保留原 host 并替换监听端口。
func replaceListenPort(value string, port int) string {
	host, _, err := splitListenAddress(value)
	if err != nil {
		return value
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
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
	data, err := renderInstanceConfigYAML(instance)
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

// renderGlobalConfigYAML 为 init 生成带字段说明的 agent 全局配置。
func renderGlobalConfigYAML(global domain.GlobalConfig) ([]byte, error) {
	data, err := yaml.Marshal(global)
	if err != nil {
		return nil, err
	}
	header := `# sbox-manager agent global config.
# version: 配置版本，目前固定为 1。
# external_host: 对外访问域名或公网 IP；不要包含 scheme、路径、query。
# paths: 受管目录，可使用相对路径，加载时会按 base-dir 解析。
# port_ranges: 自动分配端口的闭区间范围，inbound/local_socks/local_http/api 分别用于入口、本地代理和 stats API。
# defaults: 新实例继承的日志、API、Clash API 和 traffic 默认值。
# security: socks/http 公开监听时的鉴权保护开关。

`
	return append([]byte(header), data...), nil
}

// renderInstanceConfigYAML 为新建或重写 instance 生成带模板说明的配置。
func renderInstanceConfigYAML(instance domain.Instance) ([]byte, error) {
	data, err := marshalEditableInstanceYAML(instance)
	if err != nil {
		return nil, err
	}
	header := `# sbox-manager instance config.
# 可用模板:
#   sboxctl add <name> --template edge    # 入口节点，默认 vmess + local socks/http + direct
#   sboxctl add <name> --template relay   # 中继节点，默认 shadowsocks + local socks/http + direct
#   sboxctl add <name> --template urltest --members edge-us,relay-us # 基于已有 instance ref 成员创建 urltest 入口节点
# 常用配置项:
# name/role/enabled: 实例名称、模板角色和启停开关。
# api: sing-box stats API；启用后 traffic 命令会读取该监听地址。
# inbounds: 客户端入口。subscription.enabled=true 时会导出到 sboxsub。
# outbounds/groups/route: 出站、选择组和默认路由。
# traffic: 当前实例参与流量统计的开关和统计维度。
# 订阅字段示例:
#   subscription:
#     enabled: true
#     user: alice
#     remark: edge-us
#     region: US

` + instanceProtocolTemplateComment() + "\n"
	return append([]byte(header), data...), nil
}

// marshalEditableInstanceYAML 输出面向用户编辑的精简 instance YAML。
func marshalEditableInstanceYAML(instance domain.Instance) ([]byte, error) {
	var node yaml.Node
	if err := node.Encode(instance); err != nil {
		return nil, err
	}
	pruneEditableYAMLNode(&node, nil)
	return yaml.Marshal(&node)
}

// pruneEditableYAMLNode 删除对当前协议无效或为空的字段，避免正文模板噪声。
func pruneEditableYAMLNode(node *yaml.Node, path []string) bool {
	if node == nil {
		return true
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return true
		}
		return pruneEditableYAMLNode(node.Content[0], path)
	case yaml.MappingNode:
		removeDefaultTagField(node)
		next := make([]*yaml.Node, 0, len(node.Content))
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			childPath := appendEditableYAMLPath(path, key.Value)
			if pruneEditableYAMLNode(value, childPath) {
				continue
			}
			next = append(next, key, value)
		}
		node.Content = next
		return len(node.Content) == 0 || isNoauthAuthNode(path, node)
	case yaml.SequenceNode:
		next := make([]*yaml.Node, 0, len(node.Content))
		for _, item := range node.Content {
			if pruneEditableYAMLNode(item, path) {
				continue
			}
			next = append(next, item)
		}
		node.Content = next
		return len(node.Content) == 0
	case yaml.ScalarNode:
		return isEditableEmptyScalar(path, node)
	default:
		return false
	}
}

// appendEditableYAMLPath 复制并追加 YAML 路径片段，避免递归时复用底层数组。
func appendEditableYAMLPath(path []string, key string) []string {
	next := make([]string, 0, len(path)+1)
	next = append(next, path...)
	return append(next, key)
}

// isEditableEmptyScalar 判断标量值是否可从可编辑 YAML 中省略。
func isEditableEmptyScalar(path []string, node *yaml.Node) bool {
	switch node.Tag {
	case "!!str":
		return node.Value == ""
	case "!!int":
		return node.Value == "0"
	case "!!bool":
		if node.Value != "false" {
			return false
		}
		return !keepFalseScalar(path)
	default:
		return false
	}
}

// keepFalseScalar 保留会影响默认值回填语义的 false 字段。
func keepFalseScalar(path []string) bool {
	if len(path) == 0 || path[len(path)-1] != "enabled" {
		return false
	}
	if len(path) == 1 {
		return true
	}
	parent := path[len(path)-2]
	return parent == "api" || parent == "traffic"
}

// removeDefaultTagField 删除可由 type/name 自动推导的默认 tag 字段。
func removeDefaultTagField(node *yaml.Node) {
	name := mappingScalarValue(node, "name")
	protocolType := mappingScalarValue(node, "type")
	tag := mappingScalarValue(node, "tag")
	if name == "" || protocolType == "" || tag != protocolType+"-"+name {
		return
	}
	removeMappingKey(node, "tag")
}

// isNoauthAuthNode 判断 auth 节点是否只是默认 noauth，可在正文中省略。
func isNoauthAuthNode(path []string, node *yaml.Node) bool {
	if len(path) == 0 || path[len(path)-1] != "auth" {
		return false
	}
	return mappingScalarValue(node, "type") == "noauth" &&
		mappingScalarValue(node, "username") == "" &&
		mappingScalarValue(node, "password") == ""
}

// mappingScalarValue 返回 mapping 中指定 key 的标量值。
func mappingScalarValue(node *yaml.Node, key string) string {
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == key && node.Content[index+1].Kind == yaml.ScalarNode {
			return node.Content[index+1].Value
		}
	}
	return ""
}

// removeMappingKey 从 YAML mapping 中删除指定 key。
func removeMappingKey(node *yaml.Node, key string) {
	next := node.Content[:0]
	for index := 0; index < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			continue
		}
		next = append(next, node.Content[index], node.Content[index+1])
	}
	node.Content = next
}

// instanceProtocolTemplateComment 返回所有受支持协议的注释模板，方便用户在新实例配置中复制启用。
func instanceProtocolTemplateComment() string {
	const template = `协议模板参考（默认全部注释，不参与加载；启用时复制到正文 inbounds/outbounds 并去掉前缀 #）。
inbounds:
  - name: vmess-main # VMess 入口。
    type: vmess
    listen: 0.0.0.0 # 监听主机，不包含端口。
    port: 24100
    tag: vmess-vmess-main # 不填默认 <type>-<name>。
    udp: true
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111 # VMess 必填 UUID。
        alter_id: 0 # 可选，默认 0。
        remark: US VMess # 可选，订阅展示名；subscription.remark 优先。
        tag: edge-us-vmess # 可选，订阅节点 tag。
    subscription:
      enabled: true # 是否导出订阅节点。
      user: alice # 必须引用 users.name。
      server: proxy.example.com # 为空时使用 global.external_host。
      remark: US VMess
      region: US
  - name: vless-main # VLESS 入口，支持 V2Ray transport。
    type: vless
    listen: 0.0.0.0
    port: 24110
    tag: vless-vless-main
    udp: true
    tls:
      enabled: true # 可按需启用 TLS。
    transport:
      type: ws # 支持 http、ws、quic、grpc、httpupgrade。
      host: proxy.example.com
      hosts: [proxy.example.com]
      path: /vless
      method: GET
      headers:
        Host: proxy.example.com
      idle_timeout: 30s
      ping_timeout: 15s
      max_early_data: 2048
      early_data_header_name: Sec-WebSocket-Protocol
      service_name: proxy
      permit_without_stream: false
    users:
      - name: alice
        uuid: 33333333-3333-4333-8333-333333333333 # VLESS 必填 UUID。
        flow: xtls-rprx-vision # 可选；当前仅支持 xtls-rprx-vision。
        remark: US VLESS
        tag: edge-us-vless
    subscription:
      enabled: true
      user: alice
      server: proxy.example.com
      remark: US VLESS
      region: US
  - name: anytls-main # AnyTLS 入口。
    type: anytls
    listen: 0.0.0.0
    port: 24120
    tag: anytls-anytls-main
    udp: true
    tls:
      enabled: true # AnyTLS 必须启用 TLS。
    users:
      - name: alice
        password: change-me # AnyTLS 用户必填密码。
    subscription:
      enabled: true
      user: alice
      server: proxy.example.com
      remark: US AnyTLS
      region: US
  - name: ss-main # Shadowsocks 入口。
    type: shadowsocks
    listen: 0.0.0.0
    port: 24200
    tag: shadowsocks-ss-main
    udp: true
    method: 2022-blake3-aes-256-gcm # 可被 users[].method 覆盖。
    users:
      - name: alice
        password: change-me-32-byte-key
        method: 2022-blake3-aes-256-gcm
    subscription:
      enabled: true
      user: alice
      server: proxy.example.com
      remark: US Shadowsocks
      region: US
  - name: local-socks # SOCKS5 本地入口；公网监听建议 password。
    type: socks5
    listen: 127.0.0.1
    port: 17000
    tag: socks5-local-socks
    udp: true
    auth:
      type: password # 支持 noauth、password。
      username: alice
      password: change-me
  - name: local-http # HTTP 本地入口；公网监听建议 password。
    type: http
    listen: 127.0.0.1
    port: 18000
    tag: http-local-http
    udp: false
    auth:
      type: password # 支持 noauth、password。
      username: alice
      password: change-me
outbounds:
  - name: direct # 直连出站，不需要 server/port。
    type: direct
  - name: block # 阻断出站，不需要 server/port。
    type: block
  - name: edge-us-local-socks # 引用已有 instance 的 socks5/http inbound。
    type: ref
    ref: edge-us.local-socks # 格式为 <instance>.<inbound>，生成时解析。
  - name: ss-upstream # Shadowsocks 上游。
    type: shadowsocks
    server: ss.example.com
    port: 443
    method: 2022-blake3-aes-256-gcm
    password: change-me
  - name: vmess-upstream # VMess 上游。
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: 22222222-2222-4222-8222-222222222222
    security: auto # 可选。
    alter_id: 0 # 可选，默认 0。
    tls:
      enabled: true
    network: tcp # VMess 底层网络，仅支持 tcp、udp。
    transport:
      type: ws # 支持 http、ws、quic、grpc、httpupgrade。
      host: vmess.example.com
      hosts: [vmess.example.com]
      path: /vmess
      method: GET
      headers:
        Host: vmess.example.com
      idle_timeout: 30s
      ping_timeout: 15s
      max_early_data: 2048
      early_data_header_name: Sec-WebSocket-Protocol
      service_name: proxy
      permit_without_stream: false
  - name: vless-upstream # VLESS 上游。
    type: vless
    server: vless.example.com
    port: 443
    uuid: 33333333-3333-4333-8333-333333333333
    flow: xtls-rprx-vision # 可选；当前仅支持 xtls-rprx-vision。
    tls:
      enabled: true
    transport:
      type: httpupgrade
      host: vless.example.com
      path: /upgrade
      method: GET
  - name: anytls-upstream # AnyTLS 上游。
    type: anytls
    server: anytls.example.com
    port: 443
    password: change-me
    tls:
      enabled: true # AnyTLS 必须启用 TLS。
  - name: trojan-upstream # Trojan 上游。
    type: trojan
    server: trojan.example.com
    port: 443
    password: change-me
    tls:
      enabled: true
  - name: hy2-upstream # Hysteria2 上游。
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: change-me
    tls:
      enabled: true
  - name: socks5-upstream # SOCKS5 上游。
    type: socks5
    server: socks.example.com
    port: 1080
    auth:
      type: password # 可为空、noauth 或 password。
      username: alice
      password: change-me
  - name: http-upstream # HTTP 上游。
    type: http
    server: http-proxy.example.com
    port: 8080
    auth:
      type: password # 可为空、noauth 或 password。
      username: alice
      password: change-me
groups:
  - name: auto # urltest group 示例，可引用上方 type=ref 的成员。
    type: urltest
    outbounds: [edge-us-local-socks]
    url: http://www.gstatic.com/generate_204
    interval: 300
    tolerance: 50`

	lines := strings.Split(strings.TrimSuffix(template, "\n"), "\n")
	var builder strings.Builder
	for _, line := range lines {
		if line == "" {
			builder.WriteString("#\n")
			continue
		}
		builder.WriteString("# ")
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	return builder.String()
}

// MemberList 返回 group 成员列表。
func MemberList(set *config.AgentConfigSet, instanceName string, groupName string) ([]string, error) {
	group, _, err := findGroup(set, instanceName, groupName)
	if err != nil {
		return nil, err
	}
	instanceValue, _, err := findInstance(set, instanceName)
	if err != nil {
		return nil, err
	}
	outbounds := make(map[string]domain.Outbound, len(instanceValue.Outbounds))
	for _, outbound := range instanceValue.Outbounds {
		outbounds[outbound.Name] = outbound
	}
	members := make([]string, 0, len(group.Outbounds))
	for _, outboundName := range group.Outbounds {
		outbound, ok := outbounds[outboundName]
		if !ok || outbound.Type != "ref" {
			members = append(members, outboundName)
			continue
		}
		memberName, _, ok := parseRefMemberWithInstances(outbound.Ref, set.Instances)
		if !ok {
			memberName, _, ok = parseRefMember(outbound.Ref)
		}
		if !ok {
			members = append(members, outboundName)
			continue
		}
		members = append(members, memberName)
	}
	return members, nil
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
	if add {
		if err := addRefMemberToInstance(&instanceValue, set.Instances, groupName, member); err != nil {
			return err
		}
	} else {
		if err := removeRefMemberFromInstance(&instanceValue, set.Instances, groupName, member); err != nil {
			return err
		}
	}
	set.Instances[instanceIndex] = instanceValue
	return WriteInstance(set.Global, set.Instances, instanceValue)
}

// parseRefMemberWithInstances 按已有 instance 名称解析 ref，支持 instance 名称包含点号。
func parseRefMemberWithInstances(ref string, instances []domain.Instance) (string, string, bool) {
	trimmed := strings.TrimSpace(ref)
	matchedName := ""
	matchedInbound := ""
	for _, instance := range instances {
		prefix := instance.Name + "."
		if !strings.HasPrefix(trimmed, prefix) || len(instance.Name) <= len(matchedName) {
			continue
		}
		inboundName := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		if inboundName == "" {
			continue
		}
		matchedName = instance.Name
		matchedInbound = inboundName
	}
	if matchedName == "" {
		return "", "", false
	}
	return matchedName, matchedInbound, true
}

// parseRefMember 从 `<instance>.<inbound>` ref 中提取 member instance 名。
func parseRefMember(ref string) (string, string, bool) {
	trimmed := strings.TrimSpace(ref)
	separator := strings.LastIndex(trimmed, ".")
	if separator <= 0 || separator == len(trimmed)-1 {
		return "", "", false
	}
	return strings.TrimSpace(trimmed[:separator]), strings.TrimSpace(trimmed[separator+1:]), true
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
