package traffic

import (
	"context"
	"fmt"
	"time"
)

// CleanupResult 表示单个周期的清理计数。
type CleanupResult struct {
	Period  string
	Count   int64
	DryRun  bool
	Cutoff  time.Time
	Deleted bool
}

// CleanupRecords 按保留期清理 traffic_records。
func CleanupRecords(ctx context.Context, repo *Repository, options Options, period string, now time.Time, location *time.Location, dryRun bool) ([]CleanupResult, error) {
	periods, err := cleanupPeriods(period)
	if err != nil {
		return nil, err
	}
	results := make([]CleanupResult, 0, len(periods))
	for _, item := range periods {
		cutoff := cleanupCutoff(item, options, now, location)
		count, err := repo.CountBefore(ctx, item, cutoff)
		if err != nil {
			return results, err
		}
		result := CleanupResult{
			Period: item,
			Count:  count,
			DryRun: dryRun,
			Cutoff: cutoff,
		}
		if !dryRun {
			deleted, err := repo.DeleteBefore(ctx, item, cutoff)
			if err != nil {
				return results, err
			}
			result.Count = deleted
			result.Deleted = true
		}
		results = append(results, result)
	}
	return results, nil
}

// cleanupPeriods 展开 cleanup period 参数。
func cleanupPeriods(period string) ([]string, error) {
	switch period {
	case "", "all":
		return []string{PeriodHourly, PeriodDaily, PeriodMonthly}, nil
	case PeriodHourly, PeriodDaily, PeriodMonthly:
		return []string{period}, nil
	default:
		return nil, fmt.Errorf("unsupported cleanup period %q", period)
	}
}

// cleanupCutoff 根据周期和保留期计算删除截止时间。
func cleanupCutoff(period string, options Options, now time.Time, location *time.Location) time.Time {
	localNow := now.In(location)
	switch period {
	case PeriodDaily:
		days := options.RetentionDays
		if days < options.DailyMinRetentionDays {
			days = options.DailyMinRetentionDays
		}
		return DayRange(localNow, location).Start.AddDate(0, 0, -days)
	case PeriodMonthly:
		months := options.MonthlyRetentionMonths
		if options.MonthlyRetentionOverride > 0 {
			months = options.MonthlyRetentionOverride
		}
		return MonthRange(localNow, location).Start.AddDate(0, -months, 0)
	default:
		return DayRange(localNow, location).Start.AddDate(0, 0, -options.RetentionDays)
	}
}
