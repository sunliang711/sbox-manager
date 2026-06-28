package traffic

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

type fakeStatsClient struct {
	responses [][]Counter
}

func (f *fakeStatsClient) Query(ctx context.Context, target Target) ([]Counter, error) {
	if len(f.responses) == 0 {
		return nil, nil
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	return next, nil
}

func TestCollectHourlyUsesBaselineDeltaAndDetectsReset(t *testing.T) {
	repo := openTrafficTestRepo(t)
	location := time.UTC
	target := Target{Instance: "edge-us", Server: "127.0.0.1:10085", Scopes: []string{ScopeUser}}
	client := &fakeStatsClient{
		responses: [][]Counter{
			{
				{Scope: ScopeUser, Name: "alice", Direction: DirectionUp, Value: 100},
				{Scope: ScopeUser, Name: "alice", Direction: DirectionDown, Value: 40},
			},
			{
				{Scope: ScopeUser, Name: "alice", Direction: DirectionUp, Value: 150},
				{Scope: ScopeUser, Name: "alice", Direction: DirectionDown, Value: 35},
			},
		},
	}
	collector := NewCollector(repo, client, location)

	first, err := collector.CollectHourly(context.Background(), []Target{target}, time.Date(2026, 6, 28, 10, 30, 0, 0, location))
	if err != nil {
		t.Fatalf("first collect hourly: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first collect records=%d want 2", len(first))
	}
	second, err := collector.CollectHourly(context.Background(), []Target{target}, time.Date(2026, 6, 28, 11, 30, 0, 0, location))
	if err != nil {
		t.Fatalf("second collect hourly: %v", err)
	}
	byDirection := map[string]Record{}
	for _, record := range second {
		byDirection[record.Direction] = record
	}
	if byDirection[DirectionUp].Bytes != 50 || byDirection[DirectionUp].ResetDetected {
		t.Fatalf("unexpected up delta: %+v", byDirection[DirectionUp])
	}
	if byDirection[DirectionDown].Bytes != 35 || !byDirection[DirectionDown].ResetDetected {
		t.Fatalf("unexpected down reset delta: %+v", byDirection[DirectionDown])
	}
}

func TestCollectDailyMonthlyAndAggregateYearlyUseStoredRecords(t *testing.T) {
	repo := openTrafficTestRepo(t)
	location := time.UTC
	collector := NewCollector(repo, nil, location)
	hourlyWindow := HourRange(time.Date(2026, 6, 28, 10, 30, 0, 0, location), location)
	records := []Record{
		testRecord("edge-us", PeriodHourly, hourlyWindow, ScopeUser, "alice", DirectionUp, 100),
		testRecord("edge-us", PeriodHourly, hourlyWindow, ScopeUser, "alice", DirectionDown, 40),
	}
	if err := repo.AddRecords(context.Background(), records); err != nil {
		t.Fatalf("seed hourly records: %v", err)
	}

	daily, err := collector.CollectDaily(context.Background(), []string{"edge-us"}, time.Date(2026, 6, 28, 12, 0, 0, 0, location))
	if err != nil {
		t.Fatalf("collect daily: %v", err)
	}
	if len(daily) != 2 {
		t.Fatalf("daily records=%d want 2", len(daily))
	}
	monthly, err := collector.CollectMonthly(context.Background(), []string{"edge-us"}, time.Date(2026, 6, 1, 0, 0, 0, 0, location))
	if err != nil {
		t.Fatalf("collect monthly: %v", err)
	}
	yearly := AggregateYearly(monthly, location)
	if len(yearly) != 2 {
		t.Fatalf("yearly records=%d want 2", len(yearly))
	}
	if yearly[0].Period != PeriodYearly || yearly[0].StartTime != "2026-01-01 00:00:00" {
		t.Fatalf("unexpected yearly aggregate: %+v", yearly[0])
	}
}

func TestCollectDailyReplacesExistingWindowWhenSourceIsEmpty(t *testing.T) {
	repo := openTrafficTestRepo(t)
	location := time.UTC
	collector := NewCollector(repo, nil, location)
	dailyWindow := DayRange(time.Date(2026, 6, 28, 12, 0, 0, 0, location), location)
	oldDaily := testRecord("edge-us", PeriodDaily, dailyWindow, ScopeUser, "alice", DirectionUp, 100)
	if err := repo.ReplaceRecords(context.Background(), []Record{oldDaily}); err != nil {
		t.Fatalf("seed daily aggregate: %v", err)
	}

	daily, err := collector.CollectDaily(context.Background(), []string{"edge-us"}, time.Date(2026, 6, 28, 12, 0, 0, 0, location))
	if err != nil {
		t.Fatalf("collect daily: %v", err)
	}
	if len(daily) != 0 {
		t.Fatalf("empty hourly source should produce no daily records, got %+v", daily)
	}
	remaining, err := repo.ListRecords(context.Background(), PeriodDaily, Filter{Instances: []string{"edge-us"}}, dailyWindow)
	if err != nil {
		t.Fatalf("list daily records: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("stale daily aggregate should be removed, remaining=%+v", remaining)
	}
}

func TestCSVExportUsesFixedHeader(t *testing.T) {
	var builder strings.Builder
	window := HourRange(time.Date(2026, 6, 28, 10, 30, 0, 0, time.UTC), time.UTC)
	if err := WriteCSV(&builder, []Record{testRecord("edge-us", PeriodHourly, window, ScopeUser, "alice", DirectionUp, 100)}); err != nil {
		t.Fatalf("write csv: %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(builder.String())).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	got := strings.Join(rows[0], ",")
	want := "instance,server,period,start_time,end_time,scope,name,direction,bytes,created_at"
	if got != want {
		t.Fatalf("csv header got %q want %q", got, want)
	}
}

func TestCleanupRecordsDryRunDoesNotDelete(t *testing.T) {
	repo := openTrafficTestRepo(t)
	location := time.UTC
	oldWindow := HourRange(time.Date(2026, 1, 1, 0, 30, 0, 0, location), location)
	record := testRecord("edge-us", PeriodHourly, oldWindow, ScopeUser, "alice", DirectionUp, 100)
	if err := repo.AddRecords(context.Background(), []Record{record}); err != nil {
		t.Fatalf("seed old record: %v", err)
	}
	options := Options{RetentionDays: 7, DailyMinRetentionDays: 62, MonthlyRetentionMonths: 36}
	results, err := CleanupRecords(context.Background(), repo, options, PeriodHourly, time.Date(2026, 6, 28, 0, 0, 0, 0, location), location, true)
	if err != nil {
		t.Fatalf("cleanup dry-run: %v", err)
	}
	if len(results) != 1 || results[0].Count != 1 || !results[0].DryRun {
		t.Fatalf("unexpected dry-run result: %+v", results)
	}
	remaining, err := repo.ListRecords(context.Background(), PeriodHourly, Filter{Instances: []string{"edge-us"}}, TimeRange{})
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("dry-run should not delete records, remaining=%d", len(remaining))
	}
}

func TestOpenRepositoryRejectsFutureSchemaVersion(t *testing.T) {
	path := t.TempDir() + "/traffic.db"
	repo, err := OpenRepository(path)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	if err := repo.db.Model(&Metadata{}).Where("id = ?", 1).Update("schema_version", trafficSchemaVersion+1).Error; err != nil {
		t.Fatalf("raise schema version: %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("close repo: %v", err)
	}
	if _, err := OpenRepository(path); err == nil {
		t.Fatal("expected future schema version error")
	}
}

func openTrafficTestRepo(t *testing.T) *Repository {
	t.Helper()
	repo, err := OpenRepository(":memory:")
	if err != nil {
		t.Fatalf("open traffic repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})
	return repo
}

func testRecord(instance string, period string, window TimeRange, scope string, name string, direction string, bytesValue uint64) Record {
	return Record{
		Instance:  instance,
		Server:    "127.0.0.1:10085",
		Period:    period,
		StartTS:   window.Start.Unix(),
		EndTS:     window.End.Unix(),
		StartTime: FormatTime(window.Start, time.UTC),
		EndTime:   FormatTime(window.End, time.UTC),
		Scope:     scope,
		Name:      name,
		Direction: direction,
		Bytes:     bytesValue,
		CreatedAt: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
	}
}
