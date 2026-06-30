package traffic

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"time"
)

var csvHeader = []string{"instance", "server", "period", "start_time", "end_time", "scope", "name", "direction", "bytes", "created_at"}

// WriteCSV 按 data-spec 固定字段导出 traffic records。
func WriteCSV(writer io.Writer, records []Record) error {
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write(csvHeader); err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}
	for _, record := range records {
		row := []string{
			record.Instance,
			record.Server,
			record.Period,
			record.StartTime,
			record.EndTime,
			record.Scope,
			record.Name,
			record.Direction,
			fmt.Sprintf("%d", record.Bytes),
			record.CreatedAt.Format(time.RFC3339),
		}
		if err := csvWriter.Write(row); err != nil {
			return fmt.Errorf("write CSV row: %w", err)
		}
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("flush CSV: %w", err)
	}
	return nil
}

// WriteCSVFile 写入 CSV 文件。
func WriteCSVFile(path string, records []Record) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create CSV file %s: %w", path, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	if err := WriteCSV(file, records); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		closed = true
		return fmt.Errorf("close CSV file %s: %w", path, err)
	}
	closed = true
	return nil
}
