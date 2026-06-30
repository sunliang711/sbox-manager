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
	"github.com/sunliang711/sbox-manager/internal/configtemplate"
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

// instanceCandidate 保存待写入的新实例以及可选的同源模板正文。
type instanceCandidate struct {
	Instance domain.Instance
	Data     []byte
}

// Init 创建标准目录和默认 config.yaml。
func Init(baseDir string, options InitOptions) error {
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("base dir cannot be empty")
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
		return fmt.Errorf("config file %s already exists; use --force to overwrite", configPath)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("check config file %s: %w", configPath, err)
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
		return fmt.Errorf("encode default config: %w", err)
	}
	if err := writeFileAtomic(configPath, data, 0640); err != nil {
		return fmt.Errorf("write config file %s: %w", configPath, err)
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
			return fmt.Errorf("create directory %s: %w", dir, err)
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
		return domain.Instance{}, fmt.Errorf("instance %q already exists", options.Name)
	}
	candidate, err := buildInstanceCandidate(set.Global, set.Instances, options)
	if err != nil {
		return domain.Instance{}, err
	}
	if err := writeInstanceCandidate(set.Global, set.Instances, candidate); err != nil {
		return domain.Instance{}, err
	}
	return candidate.Instance, nil
}

// buildInstanceCandidate 构造新增实例，并为内置模板保留同源渲染正文。
func buildInstanceCandidate(global domain.GlobalConfig, existing []domain.Instance, options AddOptions) (instanceCandidate, error) {
	var instance domain.Instance
	var templateName string
	if options.FromFile != "" {
		if len(options.Members) > 0 {
			return instanceCandidate{}, fmt.Errorf("--members can only be used with the built-in urltest template")
		}
		loaded, err := loadInstanceDraft(options.FromFile, global)
		if err != nil {
			return instanceCandidate{}, err
		}
		instance = loaded
		instance.Name = options.Name
	} else {
		templateName = firstNonEmpty(options.Template, "edge")
		created, err := templateInstance(global, existing, options.Name, options.Template, options.Members)
		if err != nil {
			return instanceCandidate{}, err
		}
		instance = created
	}
	if options.AllocatePorts && !options.KeepTemplatePorts {
		if err := allocatePorts(global, existing, &instance); err != nil {
			return instanceCandidate{}, err
		}
	}
	domain.ApplyInstanceDefaults(&instance)
	if err := domain.ValidateInstance(global, &instance); err != nil {
		return instanceCandidate{}, err
	}
	candidate := instanceCandidate{Instance: instance}
	if templateName != "" {
		data, err := renderInstanceFromTemplate(instance, templateName)
		if err != nil {
			return instanceCandidate{}, err
		}
		candidate.Data = data
	}
	return candidate, nil
}

func templateInstance(global domain.GlobalConfig, existing []domain.Instance, name string, template string, members []string) (domain.Instance, error) {
	templateName := firstNonEmpty(template, "edge")
	context, err := templateContextForAdd(name, templateName, existing, members)
	if err != nil {
		return domain.Instance{}, err
	}
	data, err := configtemplate.RenderInstance(templateName, context)
	if err != nil {
		return domain.Instance{}, err
	}
	instance := domain.DefaultInstance(global)
	if err := config.DecodeStrict(data, "yaml", "Instance template", &instance); err != nil {
		return domain.Instance{}, err
	}
	domain.ApplyInstanceDefaults(&instance)
	return instance, nil
}

// templateContextForAdd 构造内置 add 模板渲染上下文，并提前校验 urltest 成员。
func templateContextForAdd(name string, templateName string, existing []domain.Instance, members []string) (configtemplate.Context, error) {
	context := configtemplate.DefaultContext()
	context.InstanceName = name
	context.Role = templateName
	context.ShowExampleFields = false
	context.IncludeProxyAuth = false
	context.SubscriptionServer = ""
	context.VmessInboundName = "vmess-main"
	context.VmessInboundPort = 24000
	context.VmessUUID = mustUUID()
	context.VmessRemark = name
	context.VmessUserTag = ""
	context.VmessOutboundName = "vmess-upstream"
	context.VmessOutboundUUID = "22222222-2222-4222-8222-222222222222"
	context.ShadowsocksInboundName = "ss-main"
	context.ShadowsocksInboundPort = 24000
	context.ShadowsocksPassword = mustRandomHex(32)
	context.ShadowsocksRemark = name
	context.LocalSocksName = "local-socks"
	context.LocalSocksPort = 17000
	context.LocalHTTPName = "local-http"
	context.LocalHTTPPort = 18000
	context.GroupName = "auto"
	context.RefMembers = nil
	context.GroupOutbounds = ""
	switch templateName {
	case "edge":
		if len(members) > 0 {
			return configtemplate.Context{}, fmt.Errorf("--members can only be used with urltest template")
		}
	case "relay":
		if len(members) > 0 {
			return configtemplate.Context{}, fmt.Errorf("--members can only be used with urltest template")
		}
	case "urltest":
		if len(members) == 0 {
			return configtemplate.Context{}, fmt.Errorf("urltest template requires existing instances via --members")
		}
		refMembers, err := templateRefMembers(existing, members)
		if err != nil {
			return configtemplate.Context{}, err
		}
		context.RefMembers = refMembers
		context.GroupOutbounds = joinRefMemberOutbounds(refMembers)
	default:
		return configtemplate.Context{}, fmt.Errorf("unsupported template %q", templateName)
	}
	return context, nil
}

// templateRefMembers 根据已有实例生成 urltest 模板的 ref 成员上下文。
func templateRefMembers(existing []domain.Instance, members []string) ([]configtemplate.RefMember, error) {
	outbounds := make([]domain.Outbound, 0, len(members))
	refMembers := make([]configtemplate.RefMember, 0, len(members))
	for _, member := range members {
		memberInstance, ok := findInstanceByName(existing, member)
		if !ok {
			return nil, fmt.Errorf("member instance %q does not exist", member)
		}
		inbound, ok := selectRefMemberInbound(memberInstance)
		if !ok {
			return nil, fmt.Errorf("member instance %q must include a socks5 or http inbound", member)
		}
		ref := refMemberRef(memberInstance.Name, inbound.Name)
		outboundName := allocateRefOutboundName(outbounds, refOutboundName(memberInstance.Name, inbound.Name), ref)
		outbounds = append(outbounds, domain.Outbound{Name: outboundName, Type: "ref", Ref: ref})
		refMembers = append(refMembers, configtemplate.RefMember{OutboundName: outboundName, RefOutboundName: outboundName, Ref: ref})
	}
	return refMembers, nil
}

// joinRefMemberOutbounds 返回 flow style list 内部使用的成员名称串。
func joinRefMemberOutbounds(members []configtemplate.RefMember) string {
	names := make([]string, 0, len(members))
	for _, member := range members {
		names = append(names, member.OutboundName)
	}
	return strings.Join(names, ", ")
}

// renderInstanceFromTemplate 使用最终 instance 值重新渲染同源模板正文。
func renderInstanceFromTemplate(instance domain.Instance, templateName string) ([]byte, error) {
	context := configTemplateContextFromInstance(instance)
	return configtemplate.RenderInstance(templateName, context)
}

// configTemplateContextFromInstance 从最终实例中提取同源模板写回需要的字段。
func configTemplateContextFromInstance(instance domain.Instance) configtemplate.Context {
	context := configtemplate.DefaultContext()
	context.InstanceName = instance.Name
	context.Role = instance.Role
	context.ShowExampleFields = false
	context.IncludeProxyAuth = false
	context.SubscriptionServer = ""
	context.GroupName = "auto"
	context.GroupOutbounds = ""
	context.RefMembers = nil
	for _, inbound := range instance.Inbounds {
		switch inbound.Name {
		case "vmess-main":
			context.VmessInboundName = inbound.Name
			context.VmessInboundPort = inbound.Port
			if len(inbound.Users) > 0 {
				context.VmessUUID = inbound.Users[0].UUID
				context.VmessUserTag = inbound.Users[0].Tag
			}
			context.VmessRemark = inbound.Subscription.Remark
			context.SubscriptionServer = inbound.Subscription.Server
		case "ss-main":
			context.ShadowsocksInboundName = inbound.Name
			context.ShadowsocksInboundPort = inbound.Port
			if len(inbound.Users) > 0 {
				context.ShadowsocksPassword = inbound.Users[0].Password
			}
			context.ShadowsocksRemark = inbound.Subscription.Remark
			context.SubscriptionServer = inbound.Subscription.Server
		case "local-socks":
			context.LocalSocksName = inbound.Name
			context.LocalSocksPort = inbound.Port
		case "local-http":
			context.LocalHTTPName = inbound.Name
			context.LocalHTTPPort = inbound.Port
		}
	}
	for _, outbound := range instance.Outbounds {
		switch outbound.Name {
		case "vmess-upstream":
			context.VmessOutboundName = outbound.Name
			context.VmessOutboundUUID = outbound.UUID
		default:
			if outbound.Type == "ref" {
				context.RefMembers = append(context.RefMembers, configtemplate.RefMember{OutboundName: outbound.Name, RefOutboundName: outbound.Name, Ref: outbound.Ref})
			}
		}
	}
	for _, group := range instance.Groups {
		if group.Type == "urltest" {
			context.GroupName = group.Name
			context.GroupOutbounds = strings.Join(group.Outbounds, ", ")
		}
	}
	if context.VmessRemark == "" {
		context.VmessRemark = instance.Name
	}
	if context.ShadowsocksRemark == "" {
		context.ShadowsocksRemark = instance.Name
	}
	return context
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
		return fmt.Errorf("instance cannot be empty")
	}
	memberInstance, ok := findInstanceByName(existing, member)
	if !ok {
		return fmt.Errorf("member instance %q does not exist", member)
	}
	if memberInstance.Name == instance.Name {
		return fmt.Errorf("member instance cannot reference itself")
	}
	inbound, ok := selectRefMemberInbound(memberInstance)
	if !ok {
		return fmt.Errorf("member instance %q must include a socks5 or http inbound", member)
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
		return fmt.Errorf("instance cannot be empty")
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
			return -1, fmt.Errorf("group %q is not selector or urltest", groupName)
		}
		return index, nil
	}
	return -1, fmt.Errorf("group %q does not exist", groupName)
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
		return domain.Instance{}, fmt.Errorf("instance %q does not exist", options.Source)
	}
	if _, exists := set.FindInstance(options.Target); exists {
		return domain.Instance{}, fmt.Errorf("instance %q already exists", options.Target)
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
	data, err := renderInstanceConfigYAML(instance)
	if err != nil {
		return fmt.Errorf("encode instance %s: %w", instance.Name, err)
	}
	return writeInstanceData(global, existing, instance, data)
}

// writeInstanceCandidate 校验并写入 add 构造出的实例候选。
func writeInstanceCandidate(global domain.GlobalConfig, existing []domain.Instance, candidate instanceCandidate) error {
	if len(candidate.Data) == 0 {
		return WriteInstance(global, existing, candidate.Instance)
	}
	data, err := renderInstanceConfigYAMLWithBody(candidate.Instance, candidate.Data)
	if err != nil {
		return fmt.Errorf("encode instance %s: %w", candidate.Instance.Name, err)
	}
	return writeInstanceData(global, existing, candidate.Instance, data)
}

// writeInstanceData 复用实例和配置集校验后写入指定 YAML 数据。
func writeInstanceData(global domain.GlobalConfig, existing []domain.Instance, instance domain.Instance, data []byte) error {
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
		return fmt.Errorf("create instances directory %s: %w", global.Paths.Instances, err)
	}
	path := filepath.Join(global.Paths.Instances, instance.Name+".yaml")
	if err := writeFileAtomic(path, data, 0640); err != nil {
		return fmt.Errorf("write instance config %s: %w", path, err)
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
		return fmt.Errorf("instance %q does not exist", name)
	}
	path, err := FindInstancePath(set.Global, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete instance config %s: %w", path, err)
	}
	if purge {
		generatedPath := filepath.Join(set.Global.Paths.Generated, "sing-box", name+".json")
		if err := os.Remove(generatedPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete generated file %s: %w", generatedPath, err)
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
			return "", fmt.Errorf("check instance config %s: %w", path, err)
		}
	}
	return "", fmt.Errorf("instance config %q does not exist", name)
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
	return renderInstanceConfigYAMLWithBody(instance, data)
}

// renderInstanceConfigYAMLWithBody 使用指定正文组装带说明和协议参考的 instance YAML。
func renderInstanceConfigYAMLWithBody(instance domain.Instance, body []byte) ([]byte, error) {
	header := renderInstanceConfigHeader(instance)
	footer := instanceProtocolTemplateComment()
	body = []byte(strings.TrimLeft(string(body), "\n"))
	result := append([]byte(header), body...)
	if !strings.HasSuffix(string(result), "\n") {
		result = append(result, '\n')
	}
	result = append(result, '\n')
	result = append(result, []byte(footer)...)
	return result, nil
}

// renderInstanceConfigHeader 生成 instance YAML 顶部的最少修改提示。
func renderInstanceConfigHeader(instance domain.Instance) string {
	roleHint := "edge/relay 通常保持 vmess-upstream；urltest 通常保持 auto。"
	switch instance.Role {
	case "edge":
		roleHint = "当前 edge 模板默认把入口流量转发到 VMess 上游 vmess-upstream。"
	case "relay":
		roleHint = "当前 relay 模板默认把入口流量转发到 VMess 上游 vmess-upstream。"
	case "urltest":
		roleHint = "当前 urltest 模板默认通过 auto group 在成员实例之间测速选择。"
	}
	return fmt.Sprintf(`# sbox-manager instance config.
# 生效配置从下方 name 字段开始；文件末尾是完整协议模板参考，默认均为注释。
# 最少需要确认:
#   - inbounds[].port: 公网入口端口；自动分配后通常可直接使用。
#   - inbounds[].users[].uuid/password: 已自动生成，复制给客户端即可；如要自定义请替换。
#   - outbounds[].server/port/uuid: 默认 VMess 上游占位值，必须改成真实服务端参数。
#   - inbounds[].subscription.remark/region: 订阅展示名和地区，可按需修改。
#   - route.default: %s
# 常用命令:
#   sboxctl validate %s
#   sboxctl render sing-box %s

`, roleHint, instance.Name, instance.Name)
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
	text, err := configtemplate.RenderProtocolReferenceComment(configtemplate.DefaultContext())
	if err != nil {
		return configtemplate.CommentBlock("协议模板参考暂不可用: " + err.Error())
	}
	return text
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
				return domain.Group{}, domain.Instance{}, fmt.Errorf("group %q is not selector or urltest", groupName)
			}
			return group, instanceValue, nil
		}
	}
	return domain.Group{}, domain.Instance{}, fmt.Errorf("group %q does not exist", groupName)
}

func findInstance(set *config.AgentConfigSet, name string) (domain.Instance, int, error) {
	if set == nil {
		return domain.Instance{}, -1, fmt.Errorf("config set cannot be empty")
	}
	for index, instance := range set.Instances {
		if instance.Name == name {
			return instance, index, nil
		}
	}
	return domain.Instance{}, -1, fmt.Errorf("instance %q does not exist", name)
}

func loadInstanceDraft(path string, global domain.GlobalConfig) (domain.Instance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Instance{}, fmt.Errorf("read instance template %s: %w", path, err)
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
		return "", fmt.Errorf("unsupported config file extension")
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
		return fmt.Errorf("editor cannot be empty")
	}
	args := append(parts[1:], path)
	command := exec.Command(parts[0], args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run editor %s: %w", parts[0], err)
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
	return "", fmt.Errorf("no available editor found; use --editor, set EDITOR, or install vim/vi/nvim/nano")
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file %s: %w", dir, err)
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
		return fmt.Errorf("set temp file permissions %s: %w", tempPath, err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("write temp file %s: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		closed = true
		return fmt.Errorf("close temp file %s: %w", tempPath, err)
	}
	closed = true
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace file %s: %w", path, err)
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
