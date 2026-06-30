package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

const (
	outputStatusOK   = "OK"
	outputStatusInfo = "INFO"
	outputStatusWarn = "WARN"
)

type outputField struct {
	Label string
	Value string
}

// outputKV 构造一行面向用户的键值字段。
func outputKV(label string, value string) outputField {
	return outputField{Label: label, Value: value}
}

// writeStatus 输出统一的状态行，并可追加缩进键值字段。
func writeStatus(cmd *cobra.Command, status string, message string, fields ...outputField) error {
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-4s %s\n", status, message); err != nil {
		return err
	}
	return writeFields(cmd, fields...)
}

// writeSectionFields 输出一个具名信息块，适用于摘要和配置检查结果。
func writeSectionFields(cmd *cobra.Command, title string, fields ...outputField) error {
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), title); err != nil {
		return err
	}
	return writeFields(cmd, fields...)
}

// writeFields 以稳定缩进和对齐方式输出键值字段。
func writeFields(cmd *cobra.Command, fields ...outputField) error {
	width := 0
	for _, field := range fields {
		if len(field.Label) > width {
			width = len(field.Label)
		}
	}
	for _, field := range fields {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "     %-*s  %s\n", width+1, field.Label+":", field.Value); err != nil {
			return err
		}
	}
	return nil
}

// writeNextSteps 输出编号式后续操作，便于用户复制命令继续执行。
func writeNextSteps(cmd *cobra.Command, steps ...string) error {
	if len(steps) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Next steps:"); err != nil {
		return err
	}
	for index, step := range steps {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", index+1, step); err != nil {
			return err
		}
	}
	return nil
}

// writeTable 输出带表头的 tab 对齐表格。
func writeTable(cmd *cobra.Command, headers []string, rows [][]string) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No items found.")
		return err
	}
	allRows := make([][]string, 0, len(rows)+1)
	allRows = append(allRows, headers)
	allRows = append(allRows, rows...)
	return writeRows(cmd, allRows)
}

// writeRows 输出已经包含表头、分隔线或数据的 tab 对齐行。
func writeRows(cmd *cobra.Command, rows [][]string) error {
	table := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintln(table, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return table.Flush()
}
