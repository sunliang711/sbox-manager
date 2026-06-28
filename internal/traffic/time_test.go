package traffic

import (
	"testing"
	"time"
)

func TestResolveRangeMonthlySupportsDayBasedFromTo(t *testing.T) {
	location := time.UTC
	timeRange, err := ResolveRange(PeriodMonthly, RangeOptions{
		From: "2026-05-01",
		To:   "2026-05-31",
	}, time.Date(2026, 6, 28, 12, 0, 0, 0, location), location)
	if err != nil {
		t.Fatalf("resolve monthly from/to: %v", err)
	}
	if got := timeRange.Start.Format(dateLayout); got != "2026-05-01" {
		t.Fatalf("start got %s want 2026-05-01", got)
	}
	if got := timeRange.End.Format(dateLayout); got != "2026-06-01" {
		t.Fatalf("end got %s want 2026-06-01", got)
	}
}

func TestResolveRangeRejectsUnsupportedFields(t *testing.T) {
	_, err := ResolveRange(PeriodHourly, RangeOptions{Month: "2026-05"}, time.Now(), time.UTC)
	if err == nil {
		t.Fatal("expected unsupported month range error")
	}
}
