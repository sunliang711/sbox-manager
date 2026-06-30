package traffic

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Collector 负责 hourly 采集和 daily/monthly 聚合。
type Collector struct {
	repo     *Repository
	client   StatsClient
	location *time.Location
}

// NewCollector 创建 traffic 采集器。
func NewCollector(repo *Repository, client StatsClient, location *time.Location) *Collector {
	return &Collector{repo: repo, client: client, location: location}
}

// CollectHourly 从 stats 当前累计值和 baseline 差值生成 hourly 记录。
func (c *Collector) CollectHourly(ctx context.Context, targets []Target, at time.Time) ([]Record, error) {
	if c.client == nil {
		return nil, fmt.Errorf("stats client cannot be empty")
	}
	window := HourRange(at, c.location)
	now := at.UTC()
	allRecords := []Record{}
	for _, target := range targets {
		counters, err := c.client.Query(ctx, target)
		if err != nil {
			return allRecords, fmt.Errorf("collect instance %s: %w", target.Instance, err)
		}
		filtered := FilterCounters(counters, target, Filter{})
		records, err := c.repo.AddHourlyCounters(ctx, target, filtered, window, now, c.location)
		if err != nil {
			return allRecords, err
		}
		allRecords = append(allRecords, records...)
	}
	return stableRecordOrder(allRecords), nil
}

// CollectDaily 从 hourly 记录聚合生成 daily 记录。
func (c *Collector) CollectDaily(ctx context.Context, instances []string, day time.Time) ([]Record, error) {
	window := DayRange(day, c.location)
	filter := Filter{Instances: instances}
	hourly, err := c.repo.ListRecords(ctx, PeriodHourly, filter, window)
	if err != nil {
		return nil, err
	}
	records := AggregateRecords(hourly, PeriodDaily, window, time.Now().UTC(), c.location)
	if err := c.repo.ReplaceWindowRecords(ctx, PeriodDaily, instances, window, records); err != nil {
		return nil, err
	}
	return records, nil
}

// CollectMonthly 从 daily 记录聚合生成 monthly 记录。
func (c *Collector) CollectMonthly(ctx context.Context, instances []string, month time.Time) ([]Record, error) {
	window := MonthRange(month, c.location)
	filter := Filter{Instances: instances}
	daily, err := c.repo.ListRecords(ctx, PeriodDaily, filter, window)
	if err != nil {
		return nil, err
	}
	records := AggregateRecords(daily, PeriodMonthly, window, time.Now().UTC(), c.location)
	if err := c.repo.ReplaceWindowRecords(ctx, PeriodMonthly, instances, window, records); err != nil {
		return nil, err
	}
	return records, nil
}

// CurrentDeltas 查询当前累计值与 baseline 的差值，但不写库也不更新 baseline。
func CurrentDeltas(ctx context.Context, repo *Repository, client StatsClient, targets []Target, at time.Time, location *time.Location, filter Filter) ([]Record, error) {
	if client == nil {
		return nil, fmt.Errorf("stats client cannot be empty")
	}
	window := HourRange(at, location)
	now := at.UTC()
	allRecords := []Record{}
	for _, target := range targets {
		counters, err := client.Query(ctx, target)
		if err != nil {
			return allRecords, fmt.Errorf("query current traffic for instance %s: %w", target.Instance, err)
		}
		filtered := FilterCounters(counters, target, filter)
		for _, counter := range filtered {
			baseline, exists, err := repo.GetBaseline(ctx, target.Instance, counter)
			if err != nil {
				return allRecords, err
			}
			delta := counter.Value
			resetDetected := false
			if exists {
				if counter.Value >= baseline {
					delta = counter.Value - baseline
				} else {
					resetDetected = true
				}
			}
			if delta == 0 {
				continue
			}
			allRecords = append(allRecords, recordFromCounter(target, PeriodCurrent, window, counter, delta, resetDetected, now, location))
		}
	}
	return stableRecordOrder(allRecords), nil
}

// AggregateRecords 将较小周期记录聚合为目标周期记录。
func AggregateRecords(records []Record, period string, window TimeRange, now time.Time, location *time.Location) []Record {
	type key struct {
		instance  string
		scope     string
		name      string
		direction string
	}
	grouped := map[key]Record{}
	for _, record := range records {
		itemKey := key{instance: record.Instance, scope: record.Scope, name: record.Name, direction: record.Direction}
		aggregate, exists := grouped[itemKey]
		if !exists {
			aggregate = Record{
				Instance:  record.Instance,
				Server:    record.Server,
				Period:    period,
				StartTS:   window.Start.Unix(),
				EndTS:     window.End.Unix(),
				StartTime: FormatTime(window.Start, location),
				EndTime:   FormatTime(window.End, location),
				Scope:     record.Scope,
				Name:      record.Name,
				Direction: record.Direction,
				CreatedAt: now,
			}
		}
		aggregate.Bytes += record.Bytes
		aggregate.ResetDetected = aggregate.ResetDetected || record.ResetDetected
		grouped[itemKey] = aggregate
	}
	result := make([]Record, 0, len(grouped))
	for _, record := range grouped {
		result = append(result, record)
	}
	return stableRecordOrder(result)
}

// AggregateYearly 将 monthly 记录动态聚合为 yearly 记录。
func AggregateYearly(records []Record, location *time.Location) []Record {
	grouped := map[string]Record{}
	now := time.Now().UTC()
	for _, record := range records {
		start := time.Unix(record.StartTS, 0).In(location)
		window := YearRange(start.Year(), location)
		key := record.Instance + "\x00" + record.Scope + "\x00" + record.Name + "\x00" + record.Direction + "\x00" + window.Start.Format(time.RFC3339)
		aggregate, exists := grouped[key]
		if !exists {
			aggregate = Record{
				Instance:  record.Instance,
				Server:    record.Server,
				Period:    PeriodYearly,
				StartTS:   window.Start.Unix(),
				EndTS:     window.End.Unix(),
				StartTime: FormatTime(window.Start, location),
				EndTime:   FormatTime(window.End, location),
				Scope:     record.Scope,
				Name:      record.Name,
				Direction: record.Direction,
				CreatedAt: now,
			}
		}
		aggregate.Bytes += record.Bytes
		aggregate.ResetDetected = aggregate.ResetDetected || record.ResetDetected
		grouped[key] = aggregate
	}
	result := make([]Record, 0, len(grouped))
	for _, record := range grouped {
		result = append(result, record)
	}
	return stableRecordOrder(result)
}

// AddInstanceSubtotal 为跨实例查询追加 instance=ALL 的小计记录。
func AddInstanceSubtotal(records []Record) []Record {
	instances := map[string]struct{}{}
	for _, record := range records {
		if record.Instance != AllInstancesName {
			instances[record.Instance] = struct{}{}
		}
	}
	if len(instances) < 2 {
		return records
	}
	type key struct {
		period    string
		startTS   int64
		endTS     int64
		startTime string
		endTime   string
		scope     string
		name      string
		direction string
	}
	grouped := map[key]Record{}
	for _, record := range records {
		if record.Instance == AllInstancesName {
			continue
		}
		itemKey := key{
			period:    record.Period,
			startTS:   record.StartTS,
			endTS:     record.EndTS,
			startTime: record.StartTime,
			endTime:   record.EndTime,
			scope:     record.Scope,
			name:      record.Name,
			direction: record.Direction,
		}
		aggregate, exists := grouped[itemKey]
		if !exists {
			aggregate = Record{
				Instance:  AllInstancesName,
				Server:    AllInstancesName,
				Period:    record.Period,
				StartTS:   record.StartTS,
				EndTS:     record.EndTS,
				StartTime: record.StartTime,
				EndTime:   record.EndTime,
				Scope:     record.Scope,
				Name:      record.Name,
				Direction: record.Direction,
				CreatedAt: record.CreatedAt,
			}
		}
		aggregate.Bytes += record.Bytes
		aggregate.ResetDetected = aggregate.ResetDetected || record.ResetDetected
		grouped[itemKey] = aggregate
	}
	result := append([]Record(nil), records...)
	for _, record := range grouped {
		result = append(result, record)
	}
	return stableRecordOrder(result)
}

// recordFromCounter 将累计计数差值转换为可落库记录。
func recordFromCounter(target Target, period string, window TimeRange, counter Counter, bytesValue uint64, resetDetected bool, createdAt time.Time, location *time.Location) Record {
	return Record{
		Instance:      target.Instance,
		Server:        target.Server,
		Period:        period,
		StartTS:       window.Start.Unix(),
		EndTS:         window.End.Unix(),
		StartTime:     FormatTime(window.Start, location),
		EndTime:       FormatTime(window.End, location),
		Scope:         counter.Scope,
		Name:          counter.Name,
		Direction:     counter.Direction,
		Bytes:         bytesValue,
		ResetDetected: resetDetected,
		CreatedAt:     createdAt,
	}
}

// stableRecordOrder 按固定字段排序记录，保证 CLI 和测试输出稳定。
func stableRecordOrder(records []Record) []Record {
	sort.Slice(records, func(i int, j int) bool {
		left := records[i]
		right := records[j]
		if left.StartTS != right.StartTS {
			return left.StartTS < right.StartTS
		}
		if left.Instance != right.Instance {
			return left.Instance < right.Instance
		}
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Direction != right.Direction {
			return left.Direction < right.Direction
		}
		return left.Bytes < right.Bytes
	})
	return records
}
