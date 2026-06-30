package traffic

import (
	"context"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"
)

// HistoryRecords 查询历史记录，yearly 会从 monthly 动态聚合。
func HistoryRecords(ctx context.Context, repo *Repository, period string, filter Filter, timeRange TimeRange, location *time.Location) ([]Record, error) {
	if period == PeriodYearly {
		monthly, err := repo.ListRecords(ctx, PeriodMonthly, filter, timeRange)
		if err != nil {
			return nil, err
		}
		return AggregateYearly(monthly, location), nil
	}
	return repo.ListRecords(ctx, period, filter, timeRange)
}

// SummaryRecord 表示按维度汇总后的流量。
type SummaryRecord struct {
	Instance  string
	Scope     string
	Name      string
	Direction string
	Bytes     uint64
}

// SummarizeRecords 按 instance/scope/name/direction 汇总记录。
func SummarizeRecords(records []Record) []SummaryRecord {
	grouped := map[string]SummaryRecord{}
	for _, record := range records {
		key := record.Instance + "\x00" + record.Scope + "\x00" + record.Name + "\x00" + record.Direction
		summary := grouped[key]
		summary.Instance = record.Instance
		summary.Scope = record.Scope
		summary.Name = record.Name
		summary.Direction = record.Direction
		summary.Bytes += record.Bytes
		grouped[key] = summary
	}
	result := make([]SummaryRecord, 0, len(grouped))
	for _, summary := range grouped {
		result = append(result, summary)
	}
	return stableSummaryOrder(result)
}

// WriteRecordsTable 输出流量记录表。
func WriteRecordsTable(writer io.Writer, records []Record) error {
	if len(records) == 0 {
		_, err := fmt.Fprintln(writer, "No traffic records found.")
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "INSTANCE\tSERVER\tPERIOD\tSTART\tEND\tSCOPE\tNAME\tDIRECTION\tBYTES\tRESET"); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%t\n",
			record.Instance,
			record.Server,
			record.Period,
			record.StartTime,
			record.EndTime,
			record.Scope,
			record.Name,
			record.Direction,
			record.Bytes,
			record.ResetDetected,
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

// WriteSummaryTable 输出流量汇总表。
func WriteSummaryTable(writer io.Writer, period string, timeRange TimeRange, instance string, scope string, name string, records []SummaryRecord) error {
	if instance == "" {
		instance = AllInstancesName
	}
	if scope == "" {
		scope = "all"
	}
	if name == "" {
		name = "all"
	}
	if _, err := fmt.Fprintln(writer, "Summary"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Period: %s\n", period); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Time range: %s to %s\n", timeRange.Start.Format(displayTimeLayout), timeRange.End.Format(displayTimeLayout)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Instance: %s\n", instance); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Scope: %s\n", scope); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "  Name: %s\n", name); err != nil {
		return err
	}
	if len(records) == 0 {
		_, err := fmt.Fprintln(writer, "No traffic records found.")
		return err
	}
	table := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "INSTANCE\tSCOPE\tNAME\tDIRECTION\tBYTES"); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%d\n",
			record.Instance,
			record.Scope,
			record.Name,
			record.Direction,
			record.Bytes,
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

// stableSummaryOrder 按固定字段排序汇总记录。
func stableSummaryOrder(records []SummaryRecord) []SummaryRecord {
	sort.Slice(records, func(i int, j int) bool {
		left := records[i]
		right := records[j]
		if left.Instance != right.Instance {
			return left.Instance < right.Instance
		}
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Direction < right.Direction
	})
	return records
}
