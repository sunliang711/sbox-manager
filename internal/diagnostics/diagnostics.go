package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
	runtimeplan "github.com/sunliang711/sbox-manager/internal/runtime"
	"github.com/sunliang711/sbox-manager/internal/service"
	"github.com/sunliang711/sbox-manager/internal/subscription"
)

const (
	// StatusOK 表示诊断项通过。
	StatusOK = "OK"
	// StatusIssue 表示诊断项发现问题。
	StatusIssue = "ISSUE"

	// FamilyAll 表示同时查询 IPv4 和 IPv6。
	FamilyAll = "all"
	// FamilyIPv4 表示只查询 IPv4。
	FamilyIPv4 = "ipv4"
	// FamilyIPv6 表示只查询 IPv6。
	FamilyIPv6 = "ipv6"
)

// DefaultIPv4Endpoints 是 ipinfo IPv4 查询的默认 fallback endpoint。
var DefaultIPv4Endpoints = []string{
	"https://api.ipify.org?format=json",
	"https://ifconfig.co/json",
}

// DefaultIPv6Endpoints 是 ipinfo IPv6 查询的默认 fallback endpoint。
var DefaultIPv6Endpoints = []string{
	"https://api6.ipify.org?format=json",
	"https://ifconfig.co/json",
}

// Check 表示一条 doctor 诊断结果。
type Check struct {
	Module  string
	Status  string
	Message string
}

// HasIssue 判断诊断结果集合中是否存在 ISSUE。
func HasIssue(checks []Check) bool {
	for _, check := range checks {
		if check.Status == StatusIssue {
			return true
		}
	}
	return false
}

// AgentDoctor 执行 sboxctl doctor 可检查的 agent 诊断项。
func AgentDoctor(ctx context.Context, baseDir string, serviceManager string) []Check {
	checks := make([]Check, 0)
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		checks = append(checks, issue("config", "agent 配置加载失败: "+err.Error()))
		return checks
	}

	checks = append(checks, checkDirectory("base-dir", set.BaseDir))
	for _, item := range managedDirs(set.Global) {
		checks = append(checks, checkDirectory(item.Module, item.Path))
	}
	checks = append(checks, checkSingBox(ctx, filepath.Join(set.Global.Paths.Bin, "sing-box"), set)...)
	checks = append(checks, checkInstanceServiceFiles(set.Instances, serviceManager)...)
	checks = append(checks, checkInstanceListeners(ctx, set.Instances)...)
	checks = append(checks, checkTrafficFiles(set.Global, serviceManager)...)
	return checks
}

// SubDoctor 执行 sboxsub doctor 可检查的订阅服务诊断项。
func SubDoctor(ctx context.Context, baseDir string, serviceManager string, listenOverride string) []Check {
	checks := make([]Check, 0)
	subConfig, err := config.LoadSubConfig(filepath.Join(baseDir, "config.yaml"), baseDir)
	if err != nil {
		checks = append(checks, issue("sub-config", "订阅服务配置加载失败: "+err.Error()))
		return checks
	}
	if listenOverride != "" {
		subConfig.Listen = listenOverride
	}
	if err := domain.ValidateSubConfig(*subConfig); err != nil {
		checks = append(checks, issue("sub-config", "订阅服务配置校验失败: "+err.Error()))
	} else {
		checks = append(checks, ok("sub-config", "订阅服务配置可加载"))
	}

	inputDir := subscription.InputsDir(baseDir)
	index, err := subscription.LoadIndexFromDir(inputDir)
	if err != nil {
		checks = append(checks, issue("sub-inputs", "订阅 inputs 加载失败: "+err.Error()))
	} else {
		checks = append(checks, ok("sub-inputs", fmt.Sprintf("sources=%d users=%d nodes=%d", len(index.Sources), index.UserCount(), len(index.Nodes))))
	}
	if subConfig.Access.Type == "none" && !isLoopbackListen(subConfig.Listen) {
		checks = append(checks, issue("sub-security", "公网监听未启用 token"))
	} else {
		checks = append(checks, ok("sub-security", "访问控制配置未发现明显问题"))
	}
	checks = append(checks, checkTCP(ctx, "sub-listen", subConfig.Listen))
	checks = append(checks, checkSubscriptionServiceFile(serviceManager))
	return checks
}

// IPInfoOptions 描述 ipinfo 查询的运行参数。
type IPInfoOptions struct {
	Family        string
	Timeout       time.Duration
	IPv4Endpoints []string
	IPv6Endpoints []string
}

// IPInfoResult 表示一个地址族的出口 IP 查询结果。
type IPInfoResult struct {
	Family   string
	IP       string
	Endpoint string
	Proxy    InstanceProxy
}

// InstanceProxy 表示从 instance inbound 选择出的本地代理入口。
type InstanceProxy struct {
	Scheme   string
	Address  string
	Username string
	Password string
}

// LookupIPInfo 通过实例本地 socks5/http listener 查询出口 IP。
func LookupIPInfo(ctx context.Context, instance domain.Instance, options IPInfoOptions) ([]IPInfoResult, error) {
	proxyValue, err := SelectInstanceProxy(instance)
	if err != nil {
		return nil, err
	}
	if options.Timeout <= 0 {
		options.Timeout = 10 * time.Second
	}
	if len(options.IPv4Endpoints) == 0 {
		options.IPv4Endpoints = DefaultIPv4Endpoints
	}
	if len(options.IPv6Endpoints) == 0 {
		options.IPv6Endpoints = DefaultIPv6Endpoints
	}

	families, err := resolveFamilies(options.Family)
	if err != nil {
		return nil, err
	}
	client, err := httpClientForProxy(proxyValue, options.Timeout)
	if err != nil {
		return nil, err
	}

	results := make([]IPInfoResult, 0, len(families))
	for _, family := range families {
		endpoints := options.IPv4Endpoints
		if family == FamilyIPv6 {
			endpoints = options.IPv6Endpoints
		}
		ip, endpoint, err := queryEndpointFallback(ctx, client, endpoints)
		if err != nil {
			return results, fmt.Errorf("%s 查询失败: %w", family, err)
		}
		results = append(results, IPInfoResult{
			Family:   family,
			IP:       ip,
			Endpoint: endpoint,
			Proxy:    proxyValue.Redacted(),
		})
	}
	return results, nil
}

// SelectInstanceProxy 从 instance 的 socks5/http inbound 中选择本地代理入口。
func SelectInstanceProxy(instance domain.Instance) (InstanceProxy, error) {
	domain.ApplyInstanceDefaults(&instance)
	for _, preferred := range []string{"socks5", "http"} {
		for _, inbound := range instance.Inbounds {
			if inbound.Type != preferred {
				continue
			}
			address, err := listenerAddress(inbound.Listen, inbound.Port)
			if err != nil {
				return InstanceProxy{}, err
			}
			selected := InstanceProxy{Scheme: preferred, Address: address}
			if inbound.Auth.Type == "password" {
				selected.Username = inbound.Auth.Username
				selected.Password = inbound.Auth.Password
			}
			return selected, nil
		}
	}
	return InstanceProxy{}, fmt.Errorf("instance %q 未配置 socks5/http 本地 listener", instance.Name)
}

// URL 返回代理入口可用于 HTTP transport 的 URL。
func (p InstanceProxy) URL() *url.URL {
	scheme := p.Scheme
	if scheme == "socks5" {
		scheme = "socks5"
	}
	value := &url.URL{Scheme: scheme, Host: p.Address}
	if p.Username != "" || p.Password != "" {
		value.User = url.UserPassword(p.Username, p.Password)
	}
	return value
}

// Redacted 返回不含敏感代理密码的副本。
func (p InstanceProxy) Redacted() InstanceProxy {
	p.Username = ""
	p.Password = ""
	return p
}

// String 返回不含敏感凭据的代理摘要。
func (p InstanceProxy) String() string {
	return p.Scheme + "://" + p.Address
}

type managedDir struct {
	Module string
	Path   string
}

// managedDirs 返回 agent 受管目录集合。
func managedDirs(global domain.GlobalConfig) []managedDir {
	return []managedDir{
		{Module: "paths.bin", Path: global.Paths.Bin},
		{Module: "paths.rules", Path: global.Paths.Rules},
		{Module: "paths.instances", Path: global.Paths.Instances},
		{Module: "paths.runtime", Path: global.Paths.Runtime},
		{Module: "paths.generated", Path: global.Paths.Generated},
		{Module: "paths.publish", Path: global.Paths.Publish},
		{Module: "paths.traffic", Path: global.Paths.Traffic},
		{Module: "paths.downloads", Path: global.Paths.Downloads},
		{Module: "paths.logs", Path: global.Paths.Logs},
	}
}

// checkDirectory 检查目录存在、类型和基础权限。
func checkDirectory(module string, target string) Check {
	info, err := os.Stat(target)
	if err != nil {
		return issue(module, fmt.Sprintf("%s 不可访问: %v", target, err))
	}
	if !info.IsDir() {
		return issue(module, fmt.Sprintf("%s 不是目录", target))
	}
	if info.Mode().Perm()&0100 == 0 {
		return issue(module, fmt.Sprintf("%s 缺少 owner execute 权限", target))
	}
	return ok(module, target)
}

// checkBinary 检查 sing-box 二进制是否可执行。
func checkBinary(binary string) Check {
	info, err := os.Stat(binary)
	if err != nil {
		return issue("sing-box.binary", fmt.Sprintf("%s 不可访问: %v", binary, err))
	}
	if info.IsDir() {
		return issue("sing-box.binary", fmt.Sprintf("%s 是目录", binary))
	}
	if info.Mode().Perm()&0111 == 0 {
		return issue("sing-box.binary", fmt.Sprintf("%s 不可执行", binary))
	}
	return ok("sing-box.binary", binary)
}

// checkSingBox 渲染运行配置并实际执行 sing-box check。
func checkSingBox(ctx context.Context, binary string, set *config.AgentConfigSet) []Check {
	if len(enabledInstances(set.Instances)) == 0 {
		if exists, err := pathExists(binary); err != nil {
			return []Check{
				issue("sing-box.binary", err.Error()),
				ok("sing-box.check", "没有启用的 instance，跳过 sing-box check"),
			}
		} else if !exists {
			return []Check{
				ok("sing-box.binary", "没有启用的 instance，未要求安装 sing-box"),
				ok("sing-box.check", "没有启用的 instance，跳过 sing-box check"),
			}
		}
		return []Check{
			checkBinary(binary),
			ok("sing-box.check", "没有启用的 instance，跳过 sing-box check"),
		}
	}
	binaryCheck := checkBinary(binary)
	if binaryCheck.Status == StatusIssue {
		return []Check{
			binaryCheck,
			issue("sing-box.check", "sing-box binary 无效，无法执行 check"),
		}
	}
	plan, err := runtimeplan.BuildPlan(set.Global, set.Instances, "")
	if err != nil {
		return []Check{
			binaryCheck,
			issue("sing-box.check", "构建 runtime plan 失败: "+err.Error()),
		}
	}
	if err := runtimeplan.CheckPlan(ctx, plan, runtimeplan.CommandConfigChecker{Binary: binary}); err != nil {
		return []Check{
			binaryCheck,
			issue("sing-box.check", err.Error()),
		}
	}
	return []Check{
		binaryCheck,
		ok("sing-box.check", "sing-box check 通过"),
	}
}

// enabledInstances 返回当前配置中启用的 instance 集合。
func enabledInstances(instances []domain.Instance) []domain.Instance {
	enabled := make([]domain.Instance, 0, len(instances))
	for _, instance := range instances {
		if instance.Enabled {
			enabled = append(enabled, instance)
		}
	}
	return enabled
}

// checkInstanceServiceFiles 检查每个启用实例对应服务文件是否存在。
func checkInstanceServiceFiles(instances []domain.Instance, managerKind string) []Check {
	kind, err := service.ResolveKind(managerKind)
	if err != nil {
		return []Check{issue("service-manager", err.Error())}
	}
	results := make([]Check, 0, len(instances))
	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}
		target := serviceFilePath(kind, instance.Name)
		results = append(results, checkFile("service."+instance.Name, target))
	}
	if len(results) == 0 {
		results = append(results, ok("service", "没有启用的 instance"))
	}
	return results
}

// checkInstanceListeners 检查 instance inbound 和 API 端口是否正在监听。
func checkInstanceListeners(ctx context.Context, instances []domain.Instance) []Check {
	results := make([]Check, 0)
	for _, instance := range instances {
		domain.ApplyInstanceDefaults(&instance)
		if !instance.Enabled {
			continue
		}
		if instance.API.Enabled {
			results = append(results, checkTCP(ctx, "api."+instance.Name, instance.API.Listen))
		}
		for _, inbound := range instance.Inbounds {
			address, err := listenerAddress(inbound.Listen, inbound.Port)
			if err != nil {
				results = append(results, issue("listen."+instance.Name+"."+inbound.Name, err.Error()))
				continue
			}
			results = append(results, checkTCP(ctx, "listen."+instance.Name+"."+inbound.Name, address))
		}
	}
	if len(results) == 0 {
		results = append(results, ok("listen", "没有启用的监听目标"))
	}
	return results
}

// checkTrafficFiles 检查 traffic DB 目录和 timer 服务文件。
func checkTrafficFiles(global domain.GlobalConfig, managerKind string) []Check {
	results := []Check{checkDirectory("traffic.dir", global.Paths.Traffic)}
	dbPath := filepath.Join(global.Paths.Traffic, "traffic.db")
	if exists, err := pathExists(dbPath); err != nil {
		results = append(results, issue("traffic.db", err.Error()))
	} else if exists {
		results = append(results, ok("traffic.db", dbPath))
	} else {
		results = append(results, ok("traffic.db", "尚未采集 traffic 数据"))
	}
	kind, err := service.ResolveKind(managerKind)
	if err != nil {
		results = append(results, issue("traffic.timer", err.Error()))
		return results
	}
	for _, period := range service.TrafficPeriods() {
		targets := trafficTimerPaths(kind, period)
		exists, err := anyPathExists(targets)
		if err != nil {
			results = append(results, issue("traffic.timer."+period, err.Error()))
			continue
		}
		if !exists {
			results = append(results, ok("traffic.timer."+period, "traffic timer 未安装；需要自动采集时执行 traffic timer install 和 traffic timer enable"))
			continue
		}
		for _, target := range targets {
			results = append(results, checkFile("traffic.timer."+period, target))
		}
	}
	return results
}

// anyPathExists 判断给定路径中是否至少有一个存在。
func anyPathExists(targets []string) (bool, error) {
	for _, target := range targets {
		exists, err := pathExists(target)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

// checkSubscriptionServiceFile 检查订阅服务文件是否存在。
func checkSubscriptionServiceFile(managerKind string) Check {
	kind, err := service.ResolveKind(managerKind)
	if err != nil {
		return issue("sub-service", err.Error())
	}
	if kind == service.KindLaunchd {
		return checkFile("sub-service", launchAgentPath(service.SubscriptionLaunchdLabel()))
	}
	return checkFile("sub-service", filepath.Join("/etc/systemd/system", service.SubscriptionSystemdServiceName()))
}

// checkFile 检查普通文件是否存在。
func checkFile(module string, target string) Check {
	info, err := os.Stat(target)
	if err != nil {
		return issue(module, fmt.Sprintf("%s 不可访问: %v", target, err))
	}
	if info.IsDir() {
		return issue(module, fmt.Sprintf("%s 是目录", target))
	}
	return ok(module, target)
}

// checkTCP 检查 TCP 地址是否可建立连接。
func checkTCP(ctx context.Context, module string, address string) Check {
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	dialCtx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", address)
	if err != nil {
		return issue(module, fmt.Sprintf("%s 不可连接: %v", address, err))
	}
	if err := conn.Close(); err != nil {
		return issue(module, fmt.Sprintf("%s 连接关闭失败: %v", address, err))
	}
	return ok(module, address)
}

// serviceFilePath 返回默认服务文件路径。
func serviceFilePath(kind string, instance string) string {
	if kind == service.KindLaunchd {
		return launchAgentPath(service.LaunchdLabel(instance))
	}
	return filepath.Join("/etc/systemd/system", service.SystemdTemplateServiceName())
}

// trafficTimerPaths 返回默认 traffic timer 文件路径集合。
func trafficTimerPaths(kind string, period string) []string {
	if kind == service.KindLaunchd {
		return []string{launchAgentPath(service.TrafficLaunchdLabel(period))}
	}
	return []string{
		filepath.Join("/etc/systemd/system", service.TrafficSystemdServiceName(period)),
		filepath.Join("/etc/systemd/system", service.TrafficSystemdTimerName(period)),
	}
}

// launchAgentPath 返回当前用户 LaunchAgents 下的 plist 路径。
func launchAgentPath(label string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("Library", "LaunchAgents", label+".plist")
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
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

// listenerAddress 将 inbound listen/port 转为本机可拨号地址。
func listenerAddress(host string, port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("listener port 必须在 1-65535 范围内")
	}
	host = strings.TrimSpace(host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", port)), nil
}

// isLoopbackListen 判断监听地址是否只绑定本机。
func isLoopbackListen(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// httpClientForProxy 构造通过指定代理访问 endpoint 的 HTTP client。
func httpClientForProxy(proxyValue InstanceProxy, timeout time.Duration) (*http.Client, error) {
	proxyURL := proxyValue.URL()
	transport := &http.Transport{}
	switch proxyValue.Scheme {
	case "http":
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5":
		dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("创建 socks5 dialer: %w", err)
		}
		transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
			if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
				return contextDialer.DialContext(ctx, network, address)
			}
			return dialWithContext(ctx, dialer, network, address)
		}
	default:
		return nil, fmt.Errorf("不支持的代理类型 %q", proxyValue.Scheme)
	}
	return &http.Client{Transport: transport, Timeout: timeout}, nil
}

// dialWithContext 为不支持 ContextDialer 的 socks dialer 补充取消控制。
func dialWithContext(ctx context.Context, dialer proxy.Dialer, network string, address string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	done := make(chan result, 1)
	go func() {
		conn, err := dialer.Dial(network, address)
		done <- result{conn: conn, err: err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-done:
		return result.conn, result.err
	}
}

// resolveFamilies 展开并校验 family 参数。
func resolveFamilies(family string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(family)) {
	case "", FamilyAll:
		return []string{FamilyIPv4, FamilyIPv6}, nil
	case FamilyIPv4:
		return []string{FamilyIPv4}, nil
	case FamilyIPv6:
		return []string{FamilyIPv6}, nil
	default:
		return nil, fmt.Errorf("family 必须是 all、ipv4 或 ipv6")
	}
}

// queryEndpointFallback 按顺序请求 endpoint，直到得到可解析 IP。
func queryEndpointFallback(ctx context.Context, client *http.Client, endpoints []string) (string, string, error) {
	if len(endpoints) == 0 {
		return "", "", fmt.Errorf("endpoint 列表不能为空")
	}
	var errs []string
	for _, endpoint := range endpoints {
		ip, err := queryEndpoint(ctx, client, endpoint)
		if err == nil {
			return ip, endpoint, nil
		}
		errs = append(errs, endpoint+": "+err.Error())
	}
	return "", "", errors.New(strings.Join(errs, "; "))
}

// queryEndpoint 请求单个 endpoint 并解析响应中的 IP。
func queryEndpoint(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	ip, err := parseIPResponse(data)
	if err != nil {
		return "", err
	}
	return ip, nil
}

// parseIPResponse 兼容 JSON 和纯文本 IP 响应。
func parseIPResponse(data []byte) (string, error) {
	text := strings.TrimSpace(string(data))
	if parsed := net.ParseIP(text); parsed != nil {
		return parsed.String(), nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err == nil {
		for _, key := range []string{"ip", "query", "origin"} {
			if value, ok := payload[key].(string); ok {
				candidate := strings.TrimSpace(strings.Split(value, ",")[0])
				if parsed := net.ParseIP(candidate); parsed != nil {
					return parsed.String(), nil
				}
			}
		}
	}
	return "", fmt.Errorf("响应中未找到 IP")
}

// ok 构造 OK 诊断项。
func ok(module string, message string) Check {
	return Check{Module: module, Status: StatusOK, Message: message}
}

// issue 构造 ISSUE 诊断项。
func issue(module string, message string) Check {
	return Check{Module: module, Status: StatusIssue, Message: message}
}

// SortChecks 按 module 排序，供测试或需要稳定输出的调用方使用。
func SortChecks(checks []Check) {
	sort.SliceStable(checks, func(i int, j int) bool {
		return checks[i].Module < checks[j].Module
	})
}
