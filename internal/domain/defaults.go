package domain

import "time"

const (
	// DefaultAgentBaseDir 是 agent 的默认工作目录。
	DefaultAgentBaseDir = "/opt/sbox-manager"
	// DefaultSubBaseDir 是订阅服务的默认工作目录。
	DefaultSubBaseDir = "/opt/sbox-sub"

	defaultVersion = 1
)

const (
	defaultLogLevel       = "info"
	defaultAPIListen      = "127.0.0.1:10085"
	defaultClashAPIListen = "127.0.0.1:19090"
	defaultSubListen      = "127.0.0.1:3003"
	defaultURLTestURL     = "http://www.gstatic.com/generate_204"
)

const (
	defaultTrafficTimezone               = "Asia/Shanghai"
	defaultTrafficRetentionDays          = 180
	defaultTrafficDailyMinRetentionDays  = 62
	defaultTrafficMonthlyRetentionMonths = 36
	defaultTrafficTimeoutSeconds         = 30
	defaultTrafficHourlyTimer            = "0 * * * *"
	defaultTrafficDailyTimer             = "10 0 * * *"
	defaultTrafficMonthlyTimer           = "30 0 1 * *"
)

// DefaultGlobalConfig 返回 agent 全局配置默认值。
func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		Version: defaultVersion,
		Paths: PathsConfig{
			Bin:       "bin",
			Rules:     "rules",
			Instances: "instances",
			Runtime:   "runtime",
			Generated: "runtime/generated",
			Publish:   "publish",
			Traffic:   "traffic",
			Downloads: "downloads",
			Logs:      "logs",
		},
		PortRanges: PortRangesConfig{
			Inbound:    PortRange{Start: 24000, End: 24999},
			LocalSocks: PortRange{Start: 17000, End: 17999},
			LocalHTTP:  PortRange{Start: 18000, End: 18999},
			API:        PortRange{Start: 10000, End: 10999},
		},
		Defaults: DefaultsConfig{
			LogLevel: defaultLogLevel,
			API: APIConfig{
				Enabled: true,
				Listen:  defaultAPIListen,
			},
			ClashAPI: APIConfig{
				Enabled: false,
				Listen:  defaultClashAPIListen,
			},
			Traffic: DefaultTrafficDefaultsConfig(),
		},
		Security: SecurityConfig{
			RequireAuthForPublicSocksHTTP: true,
			AllowNoauthPublic:             false,
		},
	}
}

// DefaultTrafficDefaultsConfig 返回全局 traffic 默认配置。
func DefaultTrafficDefaultsConfig() TrafficDefaultsConfig {
	return TrafficDefaultsConfig{
		Enabled:                true,
		Timezone:               defaultTrafficTimezone,
		RetentionDays:          defaultTrafficRetentionDays,
		DailyMinRetentionDays:  defaultTrafficDailyMinRetentionDays,
		MonthlyRetentionMonths: defaultTrafficMonthlyRetentionMonths,
		TimeoutSeconds:         defaultTrafficTimeoutSeconds,
		Timer: TrafficTimerConfig{
			Hourly:  defaultTrafficHourlyTimer,
			Daily:   defaultTrafficDailyTimer,
			Monthly: defaultTrafficMonthlyTimer,
		},
	}
}

// DefaultTrafficConfig 返回独立 traffic 配置默认值。
func DefaultTrafficConfig() TrafficConfig {
	defaults := DefaultTrafficDefaultsConfig()
	return TrafficConfig{
		Version:                defaultVersion,
		Enabled:                defaults.Enabled,
		Timezone:               defaults.Timezone,
		RetentionDays:          defaults.RetentionDays,
		DailyMinRetentionDays:  defaults.DailyMinRetentionDays,
		MonthlyRetentionMonths: defaults.MonthlyRetentionMonths,
		TimeoutSeconds:         defaults.TimeoutSeconds,
		Timer:                  defaults.Timer,
	}
}

// DefaultInstance 返回继承全局默认值的 instance 配置骨架。
func DefaultInstance(global GlobalConfig) Instance {
	return Instance{
		Enabled: true,
		Role:    "edge",
		Labels:  []string{},
		API:     global.Defaults.API,
		Traffic: InstanceTrafficConfig{
			Enabled: global.Defaults.Traffic.Enabled,
			Scopes:  []string{"user", "inbound", "outbound"},
		},
	}
}

// DefaultInbound 返回指定名称和类型的 inbound 默认值。
func DefaultInbound(name string, inboundType string) Inbound {
	inbound := Inbound{
		Name:   name,
		Type:   inboundType,
		Listen: "0.0.0.0",
		Auth: AuthConfig{
			Type: "noauth",
		},
		Subscription: SubscriptionConfig{
			Enabled: false,
		},
	}
	ApplyInboundDefaults(&inbound)
	return inbound
}

// DefaultGroup 返回指定名称和类型的 group 默认值。
func DefaultGroup(name string, groupType string) Group {
	group := Group{
		Name: name,
		Type: groupType,
	}
	ApplyGroupDefaults(&group)
	return group
}

// DefaultSubConfig 返回订阅服务默认配置。
func DefaultSubConfig() SubConfig {
	return SubConfig{
		Version: defaultVersion,
		Listen:  defaultSubListen,
		Access: AccessConfig{
			Type: "none",
		},
		TemplatesDir:  "templates",
		WatchInterval: 2 * time.Second,
		WatchDebounce: 300 * time.Millisecond,
		ManagedConfig: ManagedConfig{
			Enabled:  true,
			Interval: 86400,
			Strict:   true,
		},
	}
}

// ApplyInstanceDefaults 补齐需要根据字段组合推导的 instance 默认值。
func ApplyInstanceDefaults(instance *Instance) {
	if instance == nil {
		return
	}
	for index := range instance.Inbounds {
		ApplyInboundDefaults(&instance.Inbounds[index])
	}
	for index := range instance.Groups {
		ApplyGroupDefaults(&instance.Groups[index])
	}
	if instance.Labels == nil {
		instance.Labels = []string{}
	}
	if instance.Traffic.Scopes == nil {
		instance.Traffic.Scopes = []string{"user", "inbound", "outbound"}
	}
}

// ApplyInboundDefaults 补齐 inbound 中依赖 name/type 的默认值。
func ApplyInboundDefaults(inbound *Inbound) {
	if inbound == nil {
		return
	}
	if inbound.Listen == "" {
		inbound.Listen = "0.0.0.0"
	}
	if inbound.Tag == "" && inbound.Name != "" && inbound.Type != "" {
		inbound.Tag = inbound.Type + "-" + inbound.Name
	}
	if inbound.Auth.Type == "" {
		inbound.Auth.Type = "noauth"
	}
}

// ApplyGroupDefaults 补齐 group 中依赖类型的默认值。
func ApplyGroupDefaults(group *Group) {
	if group == nil {
		return
	}
	if group.Type == "urltest" {
		if group.URL == "" {
			group.URL = defaultURLTestURL
		}
		if group.Interval == 0 {
			group.Interval = 300
		}
	}
}
