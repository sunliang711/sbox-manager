package domain

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var safeNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidateGlobalConfig 校验 agent 全局配置。
func ValidateGlobalConfig(config GlobalConfig) error {
	var errs ValidationErrors

	if config.Version != defaultVersion {
		errs.Add("version", "只能为 1")
	}
	if err := validateExternalHost(config.ExternalHost); err != nil {
		errs.Add("external_host", "%v", err)
	}
	validatePaths(config.Paths, &errs)
	validatePortRanges(config.PortRanges, &errs)
	if !allowedValue(config.Defaults.LogLevel, "trace", "debug", "info", "warn", "error") {
		errs.Add("defaults.log_level", "不支持的日志级别 %q", config.Defaults.LogLevel)
	}
	validateAPIConfig("defaults.api", config.Defaults.API, &errs)
	validateAPIConfig("defaults.clash_api", config.Defaults.ClashAPI, &errs)
	validateTrafficDefaults("defaults.traffic", config.Defaults.Traffic, &errs)
	return errs.ErrOrNil()
}

// ValidateInstance 校验单个 instance 配置。
func ValidateInstance(global GlobalConfig, instance *Instance) error {
	var errs ValidationErrors
	if instance == nil {
		errs.Add("instance", "不能为空")
		return errs.ErrOrNil()
	}

	ApplyInstanceDefaults(instance)
	validateSafeName("name", instance.Name, &errs)
	if instance.Name == "ALL" {
		errs.Add("name", "ALL 是保留名称")
	}
	if !allowedValue(instance.Role, "edge", "relay", "urltest") {
		errs.Add("role", "不支持的 role %q", instance.Role)
	}
	validateAPIConfig("api", instance.API, &errs)
	validateInbounds(global, instance.Inbounds, &errs)
	outboundNames := validateOutbounds(instance.Outbounds, &errs)
	groupNames := validateGroups(instance.Groups, outboundNames, &errs)
	validateRoute(instance.Route, outboundNames, groupNames, &errs)
	validateInstanceTraffic(instance.Traffic, &errs)
	return errs.ErrOrNil()
}

// ValidateConfigSet 校验全局配置和一组 instance 的组合约束。
func ValidateConfigSet(global GlobalConfig, instances []Instance) error {
	var errs ValidationErrors
	mergeValidationError(ValidateGlobalConfig(global), &errs)

	for index := range instances {
		mergeValidationError(ValidateInstance(global, &instances[index]), &errs)
	}
	validatePortConflicts(instances, &errs)
	return errs.ErrOrNil()
}

// ValidateSubConfig 校验订阅服务配置。
func ValidateSubConfig(config SubConfig) error {
	var errs ValidationErrors
	if config.Version != defaultVersion {
		errs.Add("version", "只能为 1")
	}
	if _, _, err := splitListenAddress(config.Listen); err != nil {
		errs.Add("listen", "%v", err)
	}
	switch config.Access.Type {
	case "none":
	case "token":
		if strings.TrimSpace(config.Access.Token) == "" {
			errs.Add("access.token", "token 模式必须配置 token")
		}
	default:
		errs.Add("access.type", "不支持的 access type %q", config.Access.Type)
	}
	if strings.TrimSpace(config.TemplatesDir) == "" {
		errs.Add("templates_dir", "不能为空")
	}
	if config.WatchInterval <= 0 {
		errs.Add("watch_interval", "必须大于 0")
	}
	if config.WatchDebounce <= 0 {
		errs.Add("watch_debounce", "必须大于 0")
	}
	if config.ManagedConfig.PublicBaseURL != "" {
		if err := validatePublicBaseURL(config.ManagedConfig.PublicBaseURL); err != nil {
			errs.Add("managed_config.public_base_url", "%v", err)
		}
	}
	if config.ManagedConfig.Interval <= 0 {
		errs.Add("managed_config.interval", "必须大于 0")
	}
	return errs.ErrOrNil()
}

// ValidateTrafficConfig 校验独立 traffic 配置。
func ValidateTrafficConfig(config TrafficConfig) error {
	var errs ValidationErrors
	if config.Version != defaultVersion {
		errs.Add("version", "只能为 1")
	}
	validateTrafficDefaults("traffic", TrafficDefaultsConfig{
		Enabled:                config.Enabled,
		Timezone:               config.Timezone,
		RetentionDays:          config.RetentionDays,
		DailyMinRetentionDays:  config.DailyMinRetentionDays,
		MonthlyRetentionMonths: config.MonthlyRetentionMonths,
		TimeoutSeconds:         config.TimeoutSeconds,
		Timer:                  config.Timer,
	}, &errs)
	return errs.ErrOrNil()
}

// mergeValidationError 将另一个聚合校验错误合并到目标错误集合。
func mergeValidationError(err error, target *ValidationErrors) {
	if err == nil {
		return
	}
	if validationErr, ok := err.(*ValidationErrors); ok {
		target.Issues = append(target.Issues, validationErr.Issues...)
		return
	}
	target.Add("validation", "%v", err)
}

// validateExternalHost 校验 external_host 不是 URL 或路径。
func validateExternalHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#") {
		return fmt.Errorf("不得包含 URL scheme、path、query 或 fragment")
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return fmt.Errorf("不得包含空白字符")
	}
	return nil
}

// validatePaths 校验目录默认值和相互关系。
func validatePaths(paths PathsConfig, errs *ValidationErrors) {
	values := map[string]string{
		"paths.bin":       paths.Bin,
		"paths.rules":     paths.Rules,
		"paths.instances": paths.Instances,
		"paths.runtime":   paths.Runtime,
		"paths.generated": paths.Generated,
		"paths.publish":   paths.Publish,
		"paths.traffic":   paths.Traffic,
		"paths.downloads": paths.Downloads,
		"paths.logs":      paths.Logs,
	}
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			errs.Add(name, "不能为空")
		}
	}

	critical := map[string]string{
		"instances": paths.Instances,
		"runtime":   paths.Runtime,
		"generated": paths.Generated,
		"publish":   paths.Publish,
		"traffic":   paths.Traffic,
		"downloads": paths.Downloads,
		"logs":      paths.Logs,
	}
	seen := make(map[string]string, len(critical))
	for name, value := range critical {
		cleaned := filepath.Clean(value)
		if other, exists := seen[cleaned]; exists {
			errs.Add("paths."+name, "不能与 paths.%s 使用同一路径", other)
			continue
		}
		seen[cleaned] = name
	}
	if !pathWithin(paths.Runtime, paths.Generated) {
		errs.Add("paths.generated", "必须位于 paths.runtime 下")
	}
}

// validatePortRanges 校验所有端口范围。
func validatePortRanges(ranges PortRangesConfig, errs *ValidationErrors) {
	validatePortRange("port_ranges.inbound", ranges.Inbound, errs)
	validatePortRange("port_ranges.local_socks", ranges.LocalSocks, errs)
	validatePortRange("port_ranges.local_http", ranges.LocalHTTP, errs)
	validatePortRange("port_ranges.api", ranges.API, errs)
}

// validatePortRange 校验单个端口范围。
func validatePortRange(path string, portRange PortRange, errs *ValidationErrors) {
	if portRange.Start < 1 || portRange.Start > 65535 {
		errs.Add(path+".start", "必须在 1-65535 范围内")
	}
	if portRange.End < 1 || portRange.End > 65535 {
		errs.Add(path+".end", "必须在 1-65535 范围内")
	}
	if portRange.Start > portRange.End {
		errs.Add(path, "start 必须小于或等于 end")
	}
}

// validateAPIConfig 校验 API 监听和非 loopback token 规则。
func validateAPIConfig(path string, api APIConfig, errs *ValidationErrors) {
	if !api.Enabled {
		return
	}
	host, _, err := splitListenAddress(api.Listen)
	if err != nil {
		errs.Add(path+".listen", "%v", err)
		return
	}
	if !isLoopbackHost(host) && strings.TrimSpace(api.Token) == "" {
		errs.Add(path+".token", "非 loopback 监听必须配置 token")
	}
}

// validateTrafficDefaults 校验 traffic 保留期、时区和定时器默认值。
func validateTrafficDefaults(path string, traffic TrafficDefaultsConfig, errs *ValidationErrors) {
	if _, err := time.LoadLocation(traffic.Timezone); err != nil {
		errs.Add(path+".timezone", "无效 IANA 时区 %q", traffic.Timezone)
	}
	if traffic.RetentionDays <= 0 {
		errs.Add(path+".retention_days", "必须大于 0")
	}
	if traffic.DailyMinRetentionDays <= 0 {
		errs.Add(path+".daily_min_retention_days", "必须大于 0")
	}
	if traffic.MonthlyRetentionMonths <= 0 {
		errs.Add(path+".monthly_retention_months", "必须大于 0")
	}
	if traffic.TimeoutSeconds <= 0 {
		errs.Add(path+".timeout_seconds", "必须大于 0")
	}
	if strings.TrimSpace(traffic.Timer.Hourly) == "" {
		errs.Add(path+".timer.hourly", "不能为空")
	}
	if strings.TrimSpace(traffic.Timer.Daily) == "" {
		errs.Add(path+".timer.daily", "不能为空")
	}
	if strings.TrimSpace(traffic.Timer.Monthly) == "" {
		errs.Add(path+".timer.monthly", "不能为空")
	}
}

// validateSafeName 校验名称可安全用于文件名和服务名。
func validateSafeName(path string, value string, errs *ValidationErrors) {
	if !safeNamePattern.MatchString(value) || value == "." || value == ".." || strings.Contains(value, "..") {
		errs.Add(path, "必须是安全 basename")
	}
}

// validateInbounds 校验所有 inbound。
func validateInbounds(global GlobalConfig, inbounds []Inbound, errs *ValidationErrors) {
	if len(inbounds) == 0 {
		errs.Add("inbounds", "至少需要一个 inbound")
		return
	}

	names := make(map[string]struct{}, len(inbounds))
	tags := make(map[string]struct{}, len(inbounds))
	for index, inbound := range inbounds {
		path := fmt.Sprintf("inbounds[%d]", index)
		validateSafeName(path+".name", inbound.Name, errs)
		if _, exists := names[inbound.Name]; exists {
			errs.Add(path+".name", "inbound 名称重复")
		}
		names[inbound.Name] = struct{}{}
		if !allowedValue(inbound.Type, "vmess", "shadowsocks", "socks5", "http") {
			errs.Add(path+".type", "不支持的 inbound type %q", inbound.Type)
			continue
		}
		if inbound.Tag != "" {
			validateSafeName(path+".tag", inbound.Tag, errs)
			if _, exists := tags[inbound.Tag]; exists {
				errs.Add(path+".tag", "inbound tag 重复")
			}
			tags[inbound.Tag] = struct{}{}
		}
		validateHost(path+".listen", inbound.Listen, errs)
		validatePort(path+".port", inbound.Port, errs)
		validateInboundAuth(global, path, inbound, errs)
		validateInboundUsers(path, inbound, errs)
		validateInboundSubscription(path, inbound, errs)
	}
}

// validateInboundAuth 校验 socks5/http 的公开监听鉴权规则。
func validateInboundAuth(global GlobalConfig, path string, inbound Inbound, errs *ValidationErrors) {
	if inbound.Type != "socks5" && inbound.Type != "http" {
		return
	}
	if !allowedValue(inbound.Auth.Type, "noauth", "password") {
		errs.Add(path+".auth.type", "不支持的 auth type %q", inbound.Auth.Type)
		return
	}
	if inbound.Auth.Type == "password" {
		if strings.TrimSpace(inbound.Auth.Username) == "" {
			errs.Add(path+".auth.username", "password 鉴权必须配置 username")
		}
		if strings.TrimSpace(inbound.Auth.Password) == "" {
			errs.Add(path+".auth.password", "password 鉴权必须配置 password")
		}
		return
	}

	// 公网 socks/http 默认必须显式启用密码鉴权，只有全局安全例外允许 noauth。
	if global.Security.RequireAuthForPublicSocksHTTP && !global.Security.AllowNoauthPublic && !isLoopbackHost(inbound.Listen) {
		errs.Add(path+".auth", "公开 socks/http inbound 默认必须启用 password 鉴权")
	}
}

// validateInboundUsers 校验 vmess/shadowsocks 用户凭据。
func validateInboundUsers(path string, inbound Inbound, errs *ValidationErrors) {
	switch inbound.Type {
	case "vmess":
		if len(inbound.Users) == 0 {
			errs.Add(path+".users", "vmess inbound 必须配置用户")
		}
		for index, user := range inbound.Users {
			userPath := fmt.Sprintf("%s.users[%d]", path, index)
			validateSafeName(userPath+".name", user.Name, errs)
			if strings.TrimSpace(user.UUID) == "" {
				errs.Add(userPath+".uuid", "vmess 用户必须配置 uuid")
			}
		}
	case "shadowsocks":
		if len(inbound.Users) == 0 {
			errs.Add(path+".users", "shadowsocks inbound 必须配置用户")
		}
		for index, user := range inbound.Users {
			userPath := fmt.Sprintf("%s.users[%d]", path, index)
			validateSafeName(userPath+".name", user.Name, errs)
			if strings.TrimSpace(user.Password) == "" {
				errs.Add(userPath+".password", "shadowsocks 用户必须配置 password")
			}
			if strings.TrimSpace(user.Method) == "" && strings.TrimSpace(inbound.Method) == "" {
				errs.Add(userPath+".method", "shadowsocks 用户必须配置 method 或继承 inbound method")
			}
		}
	}
}

// validateInboundSubscription 校验订阅导出配置引用的用户。
func validateInboundSubscription(path string, inbound Inbound, errs *ValidationErrors) {
	if !inbound.Subscription.Enabled {
		return
	}
	if strings.TrimSpace(inbound.Subscription.User) == "" {
		errs.Add(path+".subscription.user", "订阅启用时必须配置 user")
	}
	if strings.TrimSpace(inbound.Subscription.Remark) == "" {
		errs.Add(path+".subscription.remark", "订阅启用时必须配置 remark")
	}
	if inbound.Subscription.Region != "" && !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(inbound.Subscription.Region) {
		errs.Add(path+".subscription.region", "必须是两位大写字母")
	}
	if inbound.Subscription.User == "" || len(inbound.Users) == 0 {
		return
	}
	for _, user := range inbound.Users {
		if user.Name == inbound.Subscription.User {
			return
		}
	}
	errs.Add(path+".subscription.user", "引用的用户 %q 不存在", inbound.Subscription.User)
}

// validateOutbounds 校验所有 outbound 并返回名称集合。
func validateOutbounds(outbounds []Outbound, errs *ValidationErrors) map[string]struct{} {
	names := make(map[string]struct{}, len(outbounds))
	for index, outbound := range outbounds {
		path := fmt.Sprintf("outbounds[%d]", index)
		validateSafeName(path+".name", outbound.Name, errs)
		if _, exists := names[outbound.Name]; exists {
			errs.Add(path+".name", "outbound 名称重复")
		}
		names[outbound.Name] = struct{}{}
		if !allowedValue(outbound.Type, "direct", "block", "shadowsocks", "vmess", "trojan", "hysteria2", "socks5", "http") {
			errs.Add(path+".type", "不支持的 outbound type %q", outbound.Type)
			continue
		}
		validateOutboundRemote(path, outbound, errs)
	}
	return names
}

// validateOutboundRemote 校验远端 outbound 的必填字段。
func validateOutboundRemote(path string, outbound Outbound, errs *ValidationErrors) {
	if outbound.Type == "direct" || outbound.Type == "block" {
		return
	}
	if strings.TrimSpace(outbound.Server) == "" {
		errs.Add(path+".server", "%s outbound 必须配置 server", outbound.Type)
	}
	validatePort(path+".port", outbound.Port, errs)
	switch outbound.Type {
	case "vmess":
		if strings.TrimSpace(outbound.UUID) == "" {
			errs.Add(path+".uuid", "vmess outbound 必须配置 uuid")
		}
	case "shadowsocks":
		if strings.TrimSpace(outbound.Method) == "" {
			errs.Add(path+".method", "shadowsocks outbound 必须配置 method")
		}
		if strings.TrimSpace(outbound.Password) == "" {
			errs.Add(path+".password", "shadowsocks outbound 必须配置 password")
		}
	case "trojan", "hysteria2":
		if strings.TrimSpace(outbound.Password) == "" {
			errs.Add(path+".password", "%s outbound 必须配置 password", outbound.Type)
		}
	case "socks5", "http":
		validateOutboundAuth(path, outbound.Auth, errs)
	}
}

// validateOutboundAuth 校验 outbound 可选认证字段。
func validateOutboundAuth(path string, auth AuthConfig, errs *ValidationErrors) {
	if auth.Type == "" {
		return
	}
	if !allowedValue(auth.Type, "noauth", "password") {
		errs.Add(path+".auth.type", "不支持的 auth type %q", auth.Type)
		return
	}
	if auth.Type == "password" {
		if strings.TrimSpace(auth.Username) == "" {
			errs.Add(path+".auth.username", "password 鉴权必须配置 username")
		}
		if strings.TrimSpace(auth.Password) == "" {
			errs.Add(path+".auth.password", "password 鉴权必须配置 password")
		}
	}
}

// validateGroups 校验 group 并返回名称集合。
func validateGroups(groups []Group, outboundNames map[string]struct{}, errs *ValidationErrors) map[string]struct{} {
	names := make(map[string]struct{}, len(groups))
	for index, group := range groups {
		path := fmt.Sprintf("groups[%d]", index)
		validateSafeName(path+".name", group.Name, errs)
		if _, exists := outboundNames[group.Name]; exists {
			errs.Add(path+".name", "不能与 outbound 同名")
		}
		if _, exists := names[group.Name]; exists {
			errs.Add(path+".name", "group 名称重复")
		}
		names[group.Name] = struct{}{}
		if !allowedValue(group.Type, "selector", "urltest") {
			errs.Add(path+".type", "不支持的 group type %q", group.Type)
			continue
		}
		if len(group.Outbounds) == 0 {
			errs.Add(path+".outbounds", "至少引用一个 outbound")
		}
		for outboundIndex, outboundName := range group.Outbounds {
			if _, exists := outboundNames[outboundName]; !exists {
				errs.Add(fmt.Sprintf("%s.outbounds[%d]", path, outboundIndex), "引用的 outbound %q 不存在", outboundName)
			}
		}
		if group.Type == "urltest" {
			if strings.TrimSpace(group.URL) == "" {
				errs.Add(path+".url", "urltest group 必须配置 url")
			}
			if group.Interval <= 0 {
				errs.Add(path+".interval", "urltest group interval 必须大于 0")
			}
		}
	}
	return names
}

// validateRoute 校验路由默认目标和规则引用。
func validateRoute(route RouteConfig, outboundNames map[string]struct{}, groupNames map[string]struct{}, errs *ValidationErrors) {
	if strings.TrimSpace(route.Default) == "" {
		errs.Add("route.default", "不能为空")
	} else if !hasRouteTarget(route.Default, outboundNames, groupNames) {
		errs.Add("route.default", "引用的 outbound/group %q 不存在", route.Default)
	}
	for index, rule := range route.Rules {
		path := fmt.Sprintf("route.rules[%d]", index)
		if !allowedValue(rule.Type, "domain", "domain_suffix", "domain_keyword", "ip_cidr", "geoip", "geosite") {
			errs.Add(path+".type", "不支持的 route rule type %q", rule.Type)
			continue
		}
		if len(rule.Values) == 0 {
			errs.Add(path+".values", "不能为空")
		}
		if strings.TrimSpace(rule.Outbound) == "" {
			errs.Add(path+".outbound", "不能为空")
		} else if !hasRouteTarget(rule.Outbound, outboundNames, groupNames) {
			errs.Add(path+".outbound", "引用的 outbound/group %q 不存在", rule.Outbound)
		}
	}
}

// validateInstanceTraffic 校验实例级 traffic scope。
func validateInstanceTraffic(traffic InstanceTrafficConfig, errs *ValidationErrors) {
	for index, scope := range traffic.Scopes {
		if !allowedValue(scope, "user", "inbound", "outbound") {
			errs.Add(fmt.Sprintf("traffic.scopes[%d]", index), "不支持的 traffic scope %q", scope)
		}
	}
}

// validatePortConflicts 校验 enabled instance 的 inbound 与 API 端口冲突。
func validatePortConflicts(instances []Instance, errs *ValidationErrors) {
	used := map[int]string{}
	for index, instance := range instances {
		if !instance.Enabled {
			continue
		}
		for inboundIndex, inbound := range instance.Inbounds {
			if inbound.Port < 1 || inbound.Port > 65535 {
				continue
			}
			owner := fmt.Sprintf("instances[%d].inbounds[%d]", index, inboundIndex)
			addPortUse(used, inbound.Port, owner, errs)
		}
		if instance.API.Enabled {
			_, port, err := splitListenAddress(instance.API.Listen)
			if err == nil {
				addPortUse(used, port, fmt.Sprintf("instances[%d].api", index), errs)
			}
		}
	}
}

// addPortUse 记录端口占用并在重复时追加冲突错误。
func addPortUse(used map[int]string, port int, owner string, errs *ValidationErrors) {
	if previous, exists := used[port]; exists {
		errs.Add(owner, "端口 %d 与 %s 冲突", port, previous)
		return
	}
	used[port] = owner
}

// hasRouteTarget 返回路由目标是否存在。
func hasRouteTarget(name string, outboundNames map[string]struct{}, groupNames map[string]struct{}) bool {
	if _, exists := outboundNames[name]; exists {
		return true
	}
	if _, exists := groupNames[name]; exists {
		return true
	}
	return false
}

// validateHost 校验 host 字段不包含端口且非空。
func validateHost(path string, host string, errs *ValidationErrors) {
	if strings.TrimSpace(host) == "" {
		errs.Add(path, "不能为空")
		return
	}
	if strings.Contains(host, "/") {
		errs.Add(path, "不能包含路径")
		return
	}
	if strings.Count(host, ":") == 1 && net.ParseIP(host) == nil {
		errs.Add(path, "必须是 host，不应包含端口")
	}
}

// validatePort 校验端口位于 1-65535。
func validatePort(path string, port int, errs *ValidationErrors) {
	if port < 1 || port > 65535 {
		errs.Add(path, "必须在 1-65535 范围内")
	}
}

// splitListenAddress 拆分并校验 HOST:PORT。
func splitListenAddress(listen string) (string, int, error) {
	host, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return "", 0, fmt.Errorf("必须是 HOST:PORT 格式: %w", err)
	}
	if strings.TrimSpace(host) == "" {
		return "", 0, fmt.Errorf("host 不能为空")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, fmt.Errorf("port 必须是 1-65535 的整数: %w", err)
	}
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port 必须在 1-65535 范围内")
	}
	return host, port, nil
}

// isLoopbackHost 判断 host 是否是 loopback。
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// pathWithin 判断 child 是否位于 parent 下且不是同一路径。
func pathWithin(parent string, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return false
	}
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return relative != "." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".."
}

// validatePublicBaseURL 校验 managed config 的公开基础 URL。
func validatePublicBaseURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 无效: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("仅允许 http 或 https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host 不能为空")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("不允许 query 或 fragment")
	}
	return nil
}

// allowedValue 判断 value 是否在候选集合内。
func allowedValue(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}
