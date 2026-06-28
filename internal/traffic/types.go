package traffic

import (
	"fmt"
	"strings"
	"time"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

const (
	// AllInstancesName 表示 traffic 命令中的全部实例目标。
	AllInstancesName = "ALL"

	// PeriodCurrent 表示当前未落库增量。
	PeriodCurrent = "current"
	// PeriodHourly 表示小时流量记录。
	PeriodHourly = "hourly"
	// PeriodDaily 表示日流量记录。
	PeriodDaily = "daily"
	// PeriodMonthly 表示月流量记录。
	PeriodMonthly = "monthly"
	// PeriodYearly 表示年流量动态聚合。
	PeriodYearly = "yearly"

	// ScopeUser 表示按用户统计。
	ScopeUser = "user"
	// ScopeInbound 表示按入站统计。
	ScopeInbound = "inbound"
	// ScopeOutbound 表示按出站统计。
	ScopeOutbound = "outbound"

	// DirectionUp 表示上行流量。
	DirectionUp = "up"
	// DirectionDown 表示下行流量。
	DirectionDown = "down"
)

var validPeriods = map[string]struct{}{
	PeriodCurrent: {},
	PeriodHourly:  {},
	PeriodDaily:   {},
	PeriodMonthly: {},
	PeriodYearly:  {},
}

var validScopes = map[string]struct{}{
	ScopeUser:     {},
	ScopeInbound:  {},
	ScopeOutbound: {},
}

// Counter 表示从 stats API 读取到的一条累计计数。
type Counter struct {
	Scope     string
	Name      string
	Direction string
	Value     uint64
}

// Target 表示一个可采集的 sing-box 实例。
type Target struct {
	Instance string
	Server   string
	Token    string
	Scopes   []string
}

// Options 表示 traffic 命令运行时配置。
type Options struct {
	DBPath                   string
	Timezone                 string
	RetentionDays            int
	DailyMinRetentionDays    int
	MonthlyRetentionMonths   int
	Timeout                  time.Duration
	MonthlyRetentionOverride int
}

// Filter 表示 show/watch/summarize/export 的维度过滤。
type Filter struct {
	Instances []string
	Scope     string
	Name      string
	Limit     int
}

// TimeRange 表示本地统计时区下的左闭右开时间范围。
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// RangeOptions 表示 CLI 传入的时间范围参数。
type RangeOptions struct {
	Date   string
	From   string
	To     string
	Days   int
	Month  string
	Months int
	Year   string
	Years  int
}

// OptionsFromGlobal 从 agent 全局配置构造 traffic 默认运行配置。
func OptionsFromGlobal(global domain.GlobalConfig) Options {
	return Options{
		DBPath:                 "",
		Timezone:               global.Defaults.Traffic.Timezone,
		RetentionDays:          global.Defaults.Traffic.RetentionDays,
		DailyMinRetentionDays:  global.Defaults.Traffic.DailyMinRetentionDays,
		MonthlyRetentionMonths: global.Defaults.Traffic.MonthlyRetentionMonths,
		Timeout:                time.Duration(global.Defaults.Traffic.TimeoutSeconds) * time.Second,
	}
}

// ApplyTrafficConfig 使用独立 traffic 配置覆盖运行配置。
func ApplyTrafficConfig(options Options, config domain.TrafficConfig) Options {
	options.Timezone = config.Timezone
	options.RetentionDays = config.RetentionDays
	options.DailyMinRetentionDays = config.DailyMinRetentionDays
	options.MonthlyRetentionMonths = config.MonthlyRetentionMonths
	options.Timeout = time.Duration(config.TimeoutSeconds) * time.Second
	return options
}

// ValidatePeriod 校验 period 是否在 traffic 支持范围内。
func ValidatePeriod(period string) error {
	if _, ok := validPeriods[period]; !ok {
		return fmt.Errorf("不支持的 period %q", period)
	}
	return nil
}

// ValidateScope 校验 scope 过滤条件是否有效。
func ValidateScope(scope string) error {
	if strings.TrimSpace(scope) == "" {
		return nil
	}
	if _, ok := validScopes[scope]; !ok {
		return fmt.Errorf("不支持的 scope %q", scope)
	}
	return nil
}

// SelectTargets 按 instance 参数选择可采集实例。
func SelectTargets(instances []domain.Instance, target string) ([]Target, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("必须指定 --instance NAME|ALL")
	}
	selected := make([]Target, 0, len(instances))
	for _, instance := range instances {
		domain.ApplyInstanceDefaults(&instance)
		if target != AllInstancesName && instance.Name != target {
			continue
		}
		if target == AllInstancesName && (!instance.Enabled || !instance.Traffic.Enabled || !instance.API.Enabled) {
			continue
		}
		if target != AllInstancesName {
			if !instance.Traffic.Enabled {
				return nil, fmt.Errorf("instance %q 未启用 traffic", instance.Name)
			}
			if !instance.API.Enabled {
				return nil, fmt.Errorf("instance %q 未启用 stats API", instance.Name)
			}
		}
		selected = append(selected, Target{
			Instance: instance.Name,
			Server:   instance.API.Listen,
			Token:    instance.API.Token,
			Scopes:   append([]string(nil), instance.Traffic.Scopes...),
		})
	}
	if len(selected) == 0 {
		if target == AllInstancesName {
			return nil, fmt.Errorf("没有可采集的 traffic instance")
		}
		return nil, fmt.Errorf("instance %q 不存在", target)
	}
	return selected, nil
}

// InstanceNames 返回 target 中的实例名集合。
func InstanceNames(targets []Target) []string {
	names := make([]string, 0, len(targets))
	for _, target := range targets {
		names = append(names, target.Instance)
	}
	return names
}

// FilterCounters 按 target scope 和查询过滤条件筛选累计计数。
func FilterCounters(counters []Counter, target Target, filter Filter) []Counter {
	scopeAllowed := make(map[string]struct{}, len(target.Scopes))
	for _, scope := range target.Scopes {
		scopeAllowed[scope] = struct{}{}
	}
	result := make([]Counter, 0, len(counters))
	for _, counter := range counters {
		if _, ok := validScopes[counter.Scope]; !ok {
			continue
		}
		if len(scopeAllowed) > 0 {
			if _, ok := scopeAllowed[counter.Scope]; !ok {
				continue
			}
		}
		if filter.Scope != "" && counter.Scope != filter.Scope {
			continue
		}
		if filter.Name != "" && counter.Name != filter.Name {
			continue
		}
		result = append(result, counter)
	}
	return result
}
