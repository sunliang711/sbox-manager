package traffic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	statscommand "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const maxStatsResponseBytes int64 = 10 << 20

// StatsClient 表示读取 sing-box stats 累计计数的客户端。
type StatsClient interface {
	Query(ctx context.Context, target Target) ([]Counter, error)
}

// V2RayStatsClient 通过 V2Ray StatsService gRPC API 读取累计计数。
type V2RayStatsClient struct {
	timeout time.Duration
}

// NewV2RayStatsClient 创建带超时的 V2Ray stats 客户端。
func NewV2RayStatsClient(timeout time.Duration) *V2RayStatsClient {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &V2RayStatsClient{timeout: timeout}
}

// Query 请求目标实例 stats 并解析累计计数。
func (c *V2RayStatsClient) Query(ctx context.Context, target Target) ([]Counter, error) {
	server, err := statsGRPCTarget(target.Server)
	if err != nil {
		return nil, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	conn, err := grpc.DialContext(
		dialCtx,
		server,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(int(maxStatsResponseBytes))),
	)
	if err != nil {
		return nil, fmt.Errorf("connect stats API %s: %w", server, err)
	}
	defer conn.Close()

	queryCtx, queryCancel := context.WithTimeout(ctx, c.timeout)
	defer queryCancel()
	if strings.TrimSpace(target.Token) != "" {
		queryCtx = metadata.AppendToOutgoingContext(queryCtx, "authorization", "Bearer "+target.Token)
	}
	response, err := statscommand.NewStatsServiceClient(conn).QueryStats(queryCtx, &statscommand.QueryStatsRequest{Reset_: false})
	if err != nil {
		return nil, fmt.Errorf("query stats API %s: %w", server, err)
	}
	counters := statsToCounters(response.GetStat())
	return FilterCounters(counters, target, Filter{}), nil
}

// ParseStatsResponse 从 stats JSON 响应中提取累计计数。
func ParseStatsResponse(data []byte) ([]Counter, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var payload interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse stats JSON: %w", err)
	}
	collected := map[string]Counter{}
	walkStatsPayload(payload, collected)
	result := make([]Counter, 0, len(collected))
	for _, counter := range collected {
		result = append(result, counter)
	}
	return result, nil
}

// statsGRPCTarget 将 listen 地址转换为 gRPC dial target。
func statsGRPCTarget(listen string) (string, error) {
	listen = strings.TrimSpace(listen)
	if listen == "" {
		return "", fmt.Errorf("stats API address cannot be empty")
	}
	if !strings.Contains(listen, "://") {
		return listen, nil
	}
	parsed, err := url.Parse(listen)
	if err != nil {
		return "", fmt.Errorf("parse stats API address: %w", err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("stats API address missing host")
	}
	return parsed.Host, nil
}

// statsToCounters 将 V2Ray StatsService 响应转换为本项目累计计数。
func statsToCounters(stats []*statscommand.Stat) []Counter {
	collected := map[string]Counter{}
	for _, stat := range stats {
		if stat == nil || stat.GetValue() < 0 {
			continue
		}
		counter, ok := statToCounter(stat.GetName(), uint64(stat.GetValue()))
		if !ok {
			continue
		}
		key := counter.Scope + "\x00" + counter.Name + "\x00" + counter.Direction
		existing := collected[key]
		counter.Value += existing.Value
		collected[key] = counter
	}
	result := make([]Counter, 0, len(collected))
	for _, counter := range collected {
		result = append(result, counter)
	}
	return result
}

// walkStatsPayload 递归遍历 stats payload，兼容 stat/stats 数组和嵌套对象。
func walkStatsPayload(value interface{}, collected map[string]Counter) {
	switch typed := value.(type) {
	case map[string]interface{}:
		if nameValue, ok := typed["name"].(string); ok {
			if rawValue, ok := typed["value"]; ok {
				if counter, ok := parseStatEntry(nameValue, rawValue); ok {
					key := counter.Scope + "\x00" + counter.Name + "\x00" + counter.Direction
					existing := collected[key]
					counter.Value += existing.Value
					collected[key] = counter
				}
			}
		}
		for _, child := range typed {
			walkStatsPayload(child, collected)
		}
	case []interface{}:
		for _, child := range typed {
			walkStatsPayload(child, collected)
		}
	}
}

// parseStatEntry 将单个 V2Ray stats name/value 转为 Counter。
func parseStatEntry(name string, rawValue interface{}) (Counter, bool) {
	scope, counterName, direction, ok := parseStatName(name)
	if !ok {
		return Counter{}, false
	}
	value, ok := parseCounterValue(rawValue)
	if !ok {
		return Counter{}, false
	}
	return Counter{
		Scope:     scope,
		Name:      counterName,
		Direction: direction,
		Value:     value,
	}, true
}

// statToCounter 将单个 stats name/value 转为 Counter。
func statToCounter(name string, value uint64) (Counter, bool) {
	scope, counterName, direction, ok := parseStatName(name)
	if !ok {
		return Counter{}, false
	}
	return Counter{
		Scope:     scope,
		Name:      counterName,
		Direction: direction,
		Value:     value,
	}, true
}

// parseStatName 解析 V2Ray stats 字段名。
func parseStatName(name string) (string, string, string, bool) {
	normalized := strings.ReplaceAll(name, "/", ">>>")
	parts := strings.Split(normalized, ">>>")
	if len(parts) < 4 {
		return "", "", "", false
	}
	scope := strings.TrimSpace(parts[0])
	counterName := strings.TrimSpace(parts[1])
	direction := normalizeDirection(parts[len(parts)-1])
	if _, ok := validScopes[scope]; !ok || counterName == "" || direction == "" {
		return "", "", "", false
	}
	return scope, counterName, direction, true
}

// normalizeDirection 将 stats 方向名转换为本项目方向值。
func normalizeDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "uplink", "up":
		return DirectionUp
	case "downlink", "down":
		return DirectionDown
	default:
		return ""
	}
}

// parseCounterValue 将 JSON 数值转为 uint64 计数。
func parseCounterValue(rawValue interface{}) (uint64, bool) {
	switch typed := rawValue.(type) {
	case json.Number:
		value, err := strconv.ParseUint(typed.String(), 10, 64)
		return value, err == nil
	case float64:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case string:
		value, err := strconv.ParseUint(typed, 10, 64)
		return value, err == nil
	default:
		return 0, false
	}
}
