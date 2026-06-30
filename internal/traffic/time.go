package traffic

import (
	"fmt"
	"strconv"
	"time"
)

const dateLayout = "2006-01-02"
const monthLayout = "2006-01"
const displayTimeLayout = "2006-01-02 15:04:05"

// LoadLocation 加载统计时区。
func LoadLocation(name string) (*time.Location, error) {
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load traffic timezone %q: %w", name, err)
	}
	return location, nil
}

// HourRange 返回指定时间所在小时的统计窗口。
func HourRange(at time.Time, location *time.Location) TimeRange {
	local := at.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, location)
	return TimeRange{Start: start, End: start.Add(time.Hour)}
}

// DayRange 返回指定日期的统计窗口。
func DayRange(day time.Time, location *time.Location) TimeRange {
	local := day.In(location)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	return TimeRange{Start: start, End: start.AddDate(0, 0, 1)}
}

// MonthRange 返回指定月份的统计窗口。
func MonthRange(month time.Time, location *time.Location) TimeRange {
	local := month.In(location)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
	return TimeRange{Start: start, End: start.AddDate(0, 1, 0)}
}

// YearRange 返回指定年份的统计窗口。
func YearRange(year int, location *time.Location) TimeRange {
	start := time.Date(year, 1, 1, 0, 0, 0, 0, location)
	return TimeRange{Start: start, End: start.AddDate(1, 0, 0)}
}

// ResolveRange 根据 period 和 CLI 范围参数计算查询窗口。
func ResolveRange(period string, options RangeOptions, now time.Time, location *time.Location) (TimeRange, error) {
	localNow := now.In(location)
	switch period {
	case PeriodHourly, PeriodDaily:
		if options.Month != "" || options.Months > 0 || options.Year != "" || options.Years > 0 {
			return TimeRange{}, fmt.Errorf("%s does not support month/months/year/years range options", period)
		}
		return resolveDayBasedRange(options, localNow, location)
	case PeriodMonthly:
		if options.Year != "" || options.Years > 0 {
			return TimeRange{}, fmt.Errorf("monthly does not support year/years range options")
		}
		if hasDayBasedRange(options) {
			return resolveDayBasedRange(options, localNow, location)
		}
		return resolveMonthRange(options, localNow, location)
	case PeriodYearly:
		if hasDayBasedRange(options) {
			return resolveDayBasedRange(options, localNow, location)
		}
		if options.Month != "" || options.Months > 0 {
			return resolveMonthRange(options, localNow, location)
		}
		return resolveYearRange(options, localNow, location)
	default:
		return TimeRange{}, fmt.Errorf("unsupported range period %q", period)
	}
}

// FormatTime 按统计时区输出周期时间字符串。
func FormatTime(value time.Time, location *time.Location) string {
	return value.In(location).Format(displayTimeLayout)
}

// ParseRFC3339InLocation 解析 RFC3339 时间并转换到统计时区。
func ParseRFC3339InLocation(value string, location *time.Location) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("time must be RFC3339: %w", err)
	}
	return parsed.In(location), nil
}

// resolveDayBasedRange 解析 date/from/to/days 形式的日期范围。
func resolveDayBasedRange(options RangeOptions, now time.Time, location *time.Location) (TimeRange, error) {
	if options.Date != "" {
		day, err := time.ParseInLocation(dateLayout, options.Date, location)
		if err != nil {
			return TimeRange{}, fmt.Errorf("date must be YYYY-MM-DD: %w", err)
		}
		return DayRange(day, location), nil
	}
	if options.Days > 0 {
		today := DayRange(now, location).Start
		start := today.AddDate(0, 0, -options.Days+1)
		return TimeRange{Start: start, End: today.AddDate(0, 0, 1)}, nil
	}
	if options.From != "" || options.To != "" {
		if options.From == "" || options.To == "" {
			return TimeRange{}, fmt.Errorf("from and to must be specified together")
		}
		start, err := time.ParseInLocation(dateLayout, options.From, location)
		if err != nil {
			return TimeRange{}, fmt.Errorf("from must be YYYY-MM-DD: %w", err)
		}
		endDay, err := time.ParseInLocation(dateLayout, options.To, location)
		if err != nil {
			return TimeRange{}, fmt.Errorf("to must be YYYY-MM-DD: %w", err)
		}
		if endDay.Before(start) {
			return TimeRange{}, fmt.Errorf("to cannot be earlier than from")
		}
		return TimeRange{Start: DayRange(start, location).Start, End: DayRange(endDay, location).End}, nil
	}
	return DayRange(now, location), nil
}

func hasDayBasedRange(options RangeOptions) bool {
	return options.Date != "" || options.From != "" || options.To != "" || options.Days > 0
}

// resolveMonthRange 解析 month/months 形式的月份范围。
func resolveMonthRange(options RangeOptions, now time.Time, location *time.Location) (TimeRange, error) {
	if options.Month != "" {
		month, err := time.ParseInLocation(monthLayout, options.Month, location)
		if err != nil {
			return TimeRange{}, fmt.Errorf("month must be YYYY-MM: %w", err)
		}
		return MonthRange(month, location), nil
	}
	months := options.Months
	if months <= 0 {
		months = 1
	}
	current := MonthRange(now, location).Start
	start := current.AddDate(0, -months+1, 0)
	return TimeRange{Start: start, End: current.AddDate(0, 1, 0)}, nil
}

// resolveYearRange 解析 year/years 形式的年份范围。
func resolveYearRange(options RangeOptions, now time.Time, location *time.Location) (TimeRange, error) {
	if options.Year != "" {
		year, err := strconv.Atoi(options.Year)
		if err != nil || year < 1 {
			return TimeRange{}, fmt.Errorf("year must be YYYY")
		}
		return YearRange(year, location), nil
	}
	years := options.Years
	if years <= 0 {
		years = 1
	}
	current := YearRange(now.Year(), location).Start
	start := current.AddDate(-years+1, 0, 0)
	return TimeRange{Start: start, End: current.AddDate(1, 0, 0)}, nil
}
