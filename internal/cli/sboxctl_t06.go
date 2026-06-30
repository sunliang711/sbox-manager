package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
	instancemgr "github.com/sunliang711/sbox-manager/internal/instance"
	"github.com/sunliang711/sbox-manager/internal/traffic"
)

var (
	newTrafficStatsClient = func(timeout time.Duration) traffic.StatsClient {
		return traffic.NewV2RayStatsClient(timeout)
	}
	trafficExecutablePath = os.Executable
)

// newSboxctlTrafficCommandT06 创建 T06 已实现的 traffic 命令树。
func newSboxctlTrafficCommandT06() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "traffic",
		Short: "Collect, query, export, and maintain traffic statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().String("db", "", "traffic statistics SQLite file")
	cmd.PersistentFlags().String("timezone", "", "statistics timezone")
	cmd.PersistentFlags().Int("retention-days", 0, "hourly/daily retention days")
	cmd.PersistentFlags().Int("timeout", 0, "request timeout in seconds")
	cmd.AddCommand(
		newTrafficCollectCommandT06(),
		newTrafficShowCommandT06(),
		newTrafficWatchCommandT06(),
		newTrafficSummarizeCommandT06(),
		newTrafficExportCommandT06(),
		newTrafficListCommandT06(),
		newTrafficCleanupCommandT06(),
		newTrafficCheckCommandT06(),
		newTrafficEditCommandT06(),
		newTrafficTimerCommandT06(),
	)
	return cmd
}

// newTrafficCollectCommandT06 创建 traffic collect 命令组。
func newTrafficCollectCommandT06() *cobra.Command {
	collect := &cobra.Command{
		Use:   "collect",
		Short: "Collect periodic traffic data",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	collect.AddCommand(
		newTrafficCollectPeriodCommand("hourly"),
		newTrafficCollectPeriodCommand("daily"),
		newTrafficCollectPeriodCommand("monthly"),
	)
	mustCommand(collect, "hourly").Flags().String("at", "", "collection time, RFC3339")
	mustCommand(collect, "daily").Flags().String("date", "", "statistics date, YYYY-MM-DD")
	mustCommand(collect, "monthly").Flags().String("month", "", "statistics month, YYYY-MM")
	return collect
}

// newTrafficCollectPeriodCommand 创建单个周期采集命令。
func newTrafficCollectPeriodCommand(period string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   period,
		Short: "Collect " + period + " traffic",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrafficCollect(cmd, period)
		},
	}
	cmd.Flags().String("instance", "", "instance name or ALL")
	return cmd
}

// newTrafficShowCommandT06 创建 traffic show 命令组。
func newTrafficShowCommandT06() *cobra.Command {
	show := &cobra.Command{
		Use:   "show",
		Short: "Query traffic statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, period := range []string{"current", "hourly", "daily", "monthly", "yearly"} {
		child := newTrafficShowPeriodCommand(period)
		show.AddCommand(child)
	}
	mustCommand(show, "hourly").Flags().String("date", "", "statistics date, YYYY-MM-DD")
	mustCommand(show, "daily").Flags().String("date", "", "statistics date, YYYY-MM-DD")
	mustCommand(show, "monthly").Flags().String("month", "", "statistics month, YYYY-MM")
	mustCommand(show, "yearly").Flags().String("year", "", "statistics year, YYYY")
	return show
}

// newTrafficShowPeriodCommand 创建单个 show 周期命令。
func newTrafficShowPeriodCommand(period string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   period,
		Short: "Query " + period + " traffic",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrafficShow(cmd, period)
		},
	}
	addTrafficQueryFlagsT06(cmd)
	return cmd
}

// newTrafficWatchCommandT06 创建 traffic watch 命令组。
func newTrafficWatchCommandT06() *cobra.Command {
	watch := &cobra.Command{
		Use:   "watch",
		Short: "Watch traffic data continuously",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	current := &cobra.Command{
		Use:   "current",
		Short: "Watch current-period traffic",
		Args:  cobra.NoArgs,
		RunE:  runTrafficWatchCurrent,
	}
	addTrafficQueryFlagsT06(current)
	current.Flags().Int("interval", 0, "refresh interval in seconds")
	current.Flags().Int("count", 0, "refresh count")
	current.Flags().Bool("no-clear", false, "do not clear the screen on refresh")
	watch.AddCommand(current)
	return watch
}

// newTrafficSummarizeCommandT06 创建 traffic summarize 命令组。
func newTrafficSummarizeCommandT06() *cobra.Command {
	summarize := &cobra.Command{
		Use:   "summarize",
		Short: "Summarize traffic statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, period := range []string{"hourly", "daily", "monthly"} {
		child := &cobra.Command{
			Use:   period,
			Short: "Summarize " + period + " traffic",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runTrafficSummarize(cmd, cmd.Name())
			},
		}
		addTrafficQueryFlagsT06(child)
		child.Flags().String("date", "", "statistics date, YYYY-MM-DD")
		summarize.AddCommand(child)
	}
	mustCommand(summarize, "monthly").Flags().String("month", "", "statistics month, YYYY-MM")
	return summarize
}

// newTrafficExportCommandT06 创建 traffic export 命令组。
func newTrafficExportCommandT06() *cobra.Command {
	export := &cobra.Command{
		Use:   "export",
		Short: "Export traffic statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, period := range []string{"hourly", "daily", "monthly"} {
		child := &cobra.Command{
			Use:   period,
			Short: "Export " + period + " traffic",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runTrafficExport(cmd, cmd.Name())
			},
		}
		addTrafficQueryFlagsT06(child)
		child.Flags().String("date", "", "statistics date, YYYY-MM-DD")
		child.Flags().String("month", "", "statistics month, YYYY-MM")
		child.Flags().String("format", "csv", "export format")
		child.Flags().String("output", "", "output file")
		export.AddCommand(child)
	}
	return export
}

// newTrafficListCommandT06 创建 traffic list 命令组。
func newTrafficListCommandT06() *cobra.Command {
	list := &cobra.Command{
		Use:   "list",
		Short: "List traffic statistic resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	list.AddCommand(&cobra.Command{
		Use:   "instances",
		Short: "List statistic instances",
		Args:  cobra.NoArgs,
		RunE:  runTrafficListInstances,
	})
	return list
}

// newTrafficCleanupCommandT06 创建 traffic cleanup 命令组。
func newTrafficCleanupCommandT06() *cobra.Command {
	cleanup := &cobra.Command{
		Use:   "cleanup",
		Short: "Clean historical traffic records",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	records := &cobra.Command{
		Use:   "records",
		Short: "Clean historical records",
		Args:  cobra.NoArgs,
		RunE:  runTrafficCleanupRecords,
	}
	records.Flags().Int("monthly-retention-months", 0, "monthly retention months")
	records.Flags().String("period", "all", "cleanup period: hourly, daily, monthly, all")
	records.Flags().Bool("dry-run", false, "preview without deleting")
	cleanup.AddCommand(records)
	return cleanup
}

// newTrafficCheckCommandT06 创建 traffic check 命令组。
func newTrafficCheckCommandT06() *cobra.Command {
	check := &cobra.Command{
		Use:   "check",
		Short: "Check traffic statistic configuration or health",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	check.AddCommand(
		&cobra.Command{Use: "config", Short: "Check traffic configuration", Args: cobra.NoArgs, RunE: runTrafficCheckConfig},
		&cobra.Command{Use: "health", Short: "Check statistic health", Args: cobra.NoArgs, RunE: runTrafficCheckHealth},
	)
	return check
}

// newTrafficEditCommandT06 创建 traffic edit 命令组。
func newTrafficEditCommandT06() *cobra.Command {
	edit := &cobra.Command{
		Use:   "edit",
		Short: "Edit traffic statistic configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	configCommand := &cobra.Command{
		Use:   "config",
		Short: "Edit traffic configuration",
		Args:  cobra.NoArgs,
		RunE:  runTrafficEditConfig,
	}
	configCommand.Flags().String("editor", "", "editor command")
	edit.AddCommand(configCommand)
	return edit
}

// newTrafficTimerCommandT06 创建 traffic timer 命令组。
func newTrafficTimerCommandT06() *cobra.Command {
	timer := &cobra.Command{
		Use:   "timer",
		Short: "Manage the traffic statistics scheduler",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, action := range []string{"install", "uninstall", "enable", "disable", "status"} {
		timer.AddCommand(&cobra.Command{
			Use:   action,
			Short: "Run timer " + action,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runTrafficTimerAction(cmd, cmd.Name())
			},
		})
	}
	logs := &cobra.Command{
		Use:   "logs",
		Short: "Show scheduler logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrafficTimerAction(cmd, "logs")
		},
	}
	logs.Flags().BoolP("follow", "f", false, "follow logs")
	run := &cobra.Command{
		Use:   "run",
		Short: "Run a statistics task immediately",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	for _, period := range []string{"hourly", "daily", "monthly"} {
		run.AddCommand(&cobra.Command{
			Use:   period,
			Short: "Run the " + period + " task immediately",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runTrafficCollectWithTarget(cmd, cmd.Name(), traffic.AllInstancesName)
			},
		})
	}
	timer.AddCommand(logs, run)
	return timer
}

// addTrafficQueryFlagsT06 为查询类 traffic 命令追加共享参数。
func addTrafficQueryFlagsT06(cmd *cobra.Command) {
	cmd.Flags().String("instance", "", "instance name or ALL")
	cmd.Flags().String("scope", "", "statistics scope: user, inbound, outbound")
	cmd.Flags().String("name", "", "scope name")
	cmd.Flags().String("from", "", "start date, YYYY-MM-DD")
	cmd.Flags().String("to", "", "end date, YYYY-MM-DD")
	cmd.Flags().Int("days", 0, "recent day count")
	cmd.Flags().Int("months", 0, "recent month count")
	cmd.Flags().Int("years", 0, "recent year count")
	cmd.Flags().Int("limit", 0, "maximum number of rows")
}

// runTrafficCollect 执行指定周期的 collect 命令。
func runTrafficCollect(cmd *cobra.Command, period string) error {
	instance, _ := cmd.Flags().GetString("instance")
	return runTrafficCollectWithTarget(cmd, period, instance)
}

// runTrafficCollectWithTarget 执行指定目标的 collect 命令。
func runTrafficCollectWithTarget(cmd *cobra.Command, period string, target string) error {
	ctx, closeRepo, err := loadTrafficCommandContext(cmd, true)
	if err != nil {
		return err
	}
	defer closeRepo()
	targets, err := traffic.SelectTargets(ctx.set.Instances, target)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return writeStatus(cmd, outputStatusInfo, "Traffic collection skipped.",
			outputKV("Period", period),
			outputKV("Reason", "No traffic-enabled instances found."),
		)
	}
	collector := traffic.NewCollector(ctx.repo, newTrafficStatsClient(ctx.options.Timeout), ctx.location)
	var records []traffic.Record
	switch period {
	case traffic.PeriodHourly:
		at := cliNow()
		if value, _ := cmd.Flags().GetString("at"); value != "" {
			at, err = traffic.ParseRFC3339InLocation(value, ctx.location)
			if err != nil {
				return err
			}
		}
		records, err = collector.CollectHourly(cmd.Context(), targets, at)
	case traffic.PeriodDaily:
		day := cliNow()
		if value, _ := cmd.Flags().GetString("date"); value != "" {
			day, err = time.ParseInLocation("2006-01-02", value, ctx.location)
			if err != nil {
				return fmt.Errorf("date must be YYYY-MM-DD: %w", err)
			}
		} else {
			day = day.In(ctx.location).AddDate(0, 0, -1)
		}
		records, err = collector.CollectDaily(cmd.Context(), traffic.InstanceNames(targets), day)
	case traffic.PeriodMonthly:
		month := cliNow()
		if value, _ := cmd.Flags().GetString("month"); value != "" {
			month, err = time.ParseInLocation("2006-01", value, ctx.location)
			if err != nil {
				return fmt.Errorf("month must be YYYY-MM: %w", err)
			}
		} else {
			month = month.In(ctx.location).AddDate(0, -1, 0)
		}
		records, err = collector.CollectMonthly(cmd.Context(), traffic.InstanceNames(targets), month)
	default:
		return fmt.Errorf("unsupported collect period %q", period)
	}
	if err != nil {
		return err
	}
	return writeStatus(cmd, outputStatusOK, "Traffic collection completed.",
		outputKV("Period", period),
		outputKV("Records", fmt.Sprintf("%d", len(records))),
	)
}

// runTrafficShow 执行 traffic show 命令。
func runTrafficShow(cmd *cobra.Command, period string) error {
	ctx, closeRepo, err := loadTrafficCommandContext(cmd, true, true)
	if err != nil {
		return err
	}
	defer closeRepo()
	var records []traffic.Record
	if period == traffic.PeriodCurrent {
		var targets []traffic.Target
		var filter traffic.Filter
		targets, filter, err = trafficTargetsAndFilter(cmd, ctx)
		if err != nil {
			return err
		}
		records, err = traffic.CurrentDeltas(cmd.Context(), ctx.repo, newTrafficStatsClient(ctx.options.Timeout), targets, cliNow(), ctx.location, filter)
	} else {
		_, filter, err := trafficInstanceNamesAndFilter(cmd, ctx)
		if err != nil {
			return err
		}
		timeRange, err := traffic.ResolveRange(period, rangeOptionsFromCommand(cmd), cliNow(), ctx.location)
		if err != nil {
			return err
		}
		records, err = traffic.HistoryRecords(cmd.Context(), ctx.repo, period, filter, timeRange, ctx.location)
		if err == nil && timeRange.Start.Before(cliNow().In(ctx.location)) && cliNow().In(ctx.location).Before(timeRange.End) {
			if currentTargets, currentErr := traffic.SelectTargets(ctx.set.Instances, instanceFlagValue(cmd)); currentErr == nil {
				current, currentErr := traffic.CurrentDeltas(cmd.Context(), ctx.repo, newTrafficStatsClient(ctx.options.Timeout), currentTargets, cliNow(), ctx.location, filter)
				if currentErr != nil {
					return currentErr
				}
				records = append(records, currentAsPeriod(current, period, cliNow(), ctx.location)...)
			}
		}
	}
	if err != nil {
		return err
	}
	if shouldAppendSubtotal(cmd) {
		records = traffic.AddInstanceSubtotal(records)
	}
	return traffic.WriteRecordsTable(cmd.OutOrStdout(), records)
}

// runTrafficWatchCurrent 执行 traffic watch current。
func runTrafficWatchCurrent(cmd *cobra.Command, args []string) error {
	interval, _ := cmd.Flags().GetInt("interval")
	if interval <= 0 {
		interval = 1
	}
	count, _ := cmd.Flags().GetInt("count")
	noClear, _ := cmd.Flags().GetBool("no-clear")
	for iteration := 0; ; iteration++ {
		if !noClear {
			if _, err := fmt.Fprint(cmd.OutOrStdout(), "\033[H\033[2J"); err != nil {
				return err
			}
		}
		if err := runTrafficShow(cmd, traffic.PeriodCurrent); err != nil {
			return err
		}
		if count > 0 && iteration+1 >= count {
			return nil
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
}

// runTrafficSummarize 执行 traffic summarize 命令。
func runTrafficSummarize(cmd *cobra.Command, period string) error {
	ctx, closeRepo, err := loadTrafficCommandContext(cmd, true, true)
	if err != nil {
		return err
	}
	defer closeRepo()
	_, filter, err := trafficInstanceNamesAndFilter(cmd, ctx)
	if err != nil {
		return err
	}
	timeRange, err := traffic.ResolveRange(period, rangeOptionsFromCommand(cmd), cliNow(), ctx.location)
	if err != nil {
		return err
	}
	records, err := traffic.HistoryRecords(cmd.Context(), ctx.repo, period, filter, timeRange, ctx.location)
	if err != nil {
		return err
	}
	if shouldAppendSubtotal(cmd) {
		records = traffic.AddInstanceSubtotal(records)
	}
	return traffic.WriteSummaryTable(cmd.OutOrStdout(), period, timeRange, instanceFlagValue(cmd), filter.Scope, filter.Name, traffic.SummarizeRecords(records))
}

// runTrafficExport 执行 traffic export 命令。
func runTrafficExport(cmd *cobra.Command, period string) error {
	ctx, closeRepo, err := loadTrafficCommandContext(cmd, true, true)
	if err != nil {
		return err
	}
	defer closeRepo()
	format, _ := cmd.Flags().GetString("format")
	if format != "" && format != "csv" {
		return fmt.Errorf("unsupported export format %q", format)
	}
	_, filter, err := trafficInstanceNamesAndFilter(cmd, ctx)
	if err != nil {
		return err
	}
	timeRange, err := traffic.ResolveRange(period, rangeOptionsFromCommand(cmd), cliNow(), ctx.location)
	if err != nil {
		return err
	}
	records, err := traffic.HistoryRecords(cmd.Context(), ctx.repo, period, filter, timeRange, ctx.location)
	if err != nil {
		return err
	}
	if shouldAppendSubtotal(cmd) {
		records = traffic.AddInstanceSubtotal(records)
	}
	output, _ := cmd.Flags().GetString("output")
	if output == "" {
		return traffic.WriteCSV(cmd.OutOrStdout(), records)
	}
	if err := traffic.WriteCSVFile(output, records); err != nil {
		return err
	}
	return writeStatus(cmd, outputStatusOK, "Traffic CSV exported.", outputKV("File", output))
}

// runTrafficListInstances 输出可采集实例列表。
func runTrafficListInstances(cmd *cobra.Command, args []string) error {
	ctx, _, err := loadTrafficCommandContext(cmd, false)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(ctx.set.Instances))
	for _, instance := range ctx.set.Instances {
		domain.ApplyInstanceDefaults(&instance)
		if !instance.Enabled || !instance.Traffic.Enabled || !instance.API.Enabled {
			continue
		}
		rows = append(rows, []string{instance.Name, instance.API.Listen, strings.Join(instance.Traffic.Scopes, ",")})
	}
	return writeTable(cmd, []string{"INSTANCE", "API", "SCOPES"}, rows)
}

// runTrafficCleanupRecords 执行 traffic cleanup records。
func runTrafficCleanupRecords(cmd *cobra.Command, args []string) error {
	ctx, closeRepo, err := loadTrafficCommandContext(cmd, true)
	if err != nil {
		return err
	}
	defer closeRepo()
	period, _ := cmd.Flags().GetString("period")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	monthlyRetention, _ := cmd.Flags().GetInt("monthly-retention-months")
	ctx.options.MonthlyRetentionOverride = monthlyRetention
	results, err := traffic.CleanupRecords(cmd.Context(), ctx.repo, ctx.options, period, cliNow(), ctx.location, dryRun)
	if err != nil {
		return err
	}
	rows := make([][]string, 0, len(results))
	for _, result := range results {
		action := "deleted"
		if result.DryRun {
			action = "would delete"
		}
		rows = append(rows, []string{result.Period, action, fmt.Sprintf("%d", result.Count), result.Cutoff.Format(time.RFC3339)})
	}
	return writeTable(cmd, []string{"PERIOD", "ACTION", "RECORDS", "CUTOFF"}, rows)
}

// runTrafficCheckConfig 校验 traffic 配置和 DB 路径。
func runTrafficCheckConfig(cmd *cobra.Command, args []string) error {
	ctx, _, err := loadTrafficCommandContext(cmd, false)
	if err != nil {
		return err
	}
	return writeStatus(cmd, outputStatusOK, "Traffic configuration is valid.",
		outputKV("DB", ctx.options.DBPath),
		outputKV("Timezone", ctx.options.Timezone),
	)
}

// runTrafficCheckHealth 检查 DB 和 stats 目标连通性。
func runTrafficCheckHealth(cmd *cobra.Command, args []string) error {
	ctx, closeRepo, err := loadTrafficCommandContext(cmd, true, true)
	if err != nil {
		return err
	}
	defer closeRepo()
	targets, err := traffic.SelectTargets(ctx.set.Instances, traffic.AllInstancesName)
	if err != nil {
		return err
	}
	client := newTrafficStatsClient(ctx.options.Timeout)
	var failed []string
	rows := make([][]string, 0, len(targets))
	for _, target := range targets {
		counters, queryErr := client.Query(cmd.Context(), target)
		if queryErr != nil {
			failed = append(failed, target.Instance)
			rows = append(rows, []string{target.Instance, "FAILED", "0", queryErr.Error()})
			continue
		}
		rows = append(rows, []string{target.Instance, "OK", fmt.Sprintf("%d", len(counters)), ""})
	}
	if err := writeTable(cmd, []string{"INSTANCE", "STATUS", "COUNTERS", "MESSAGE"}, rows); err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("traffic health check failed: %s", strings.Join(failed, ","))
	}
	return nil
}

// runTrafficEditConfig 编辑独立 traffic 配置文件。
func runTrafficEditConfig(cmd *cobra.Command, args []string) error {
	ctx, _, err := loadTrafficCommandContext(cmd, false)
	if err != nil {
		return err
	}
	editor, _ := cmd.Flags().GetString("editor")
	path := trafficConfigPath(ctx.set.Global)
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return fmt.Errorf("create traffic config directory: %w", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		data, err := yaml.Marshal(trafficConfigFromDefaults(ctx.set.Global.Defaults.Traffic))
		if err != nil {
			return fmt.Errorf("generate default traffic config: %w", err)
		}
		if err := os.WriteFile(path, data, 0640); err != nil {
			return fmt.Errorf("write default traffic config: %w", err)
		}
	}
	draft := draftPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read traffic config: %w", err)
	}
	if err := os.WriteFile(draft, data, 0600); err != nil {
		return fmt.Errorf("write traffic draft: %w", err)
	}
	defer os.Remove(draft)
	if err := instancemgr.EditFileWithCommand(draft, editor); err != nil {
		return err
	}
	if _, err := config.LoadTrafficConfig(draft); err != nil {
		return err
	}
	if err := os.Rename(draft, path); err != nil {
		return fmt.Errorf("replace traffic config: %w", err)
	}
	return writeStatus(cmd, outputStatusOK, "Traffic configuration updated.", outputKV("File", path))
}

// runTrafficTimerAction 执行 traffic timer 管理动作。
func runTrafficTimerAction(cmd *cobra.Command, action string) error {
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	set, err := config.LoadAgentConfigSet(options.baseDir)
	if err != nil {
		return err
	}
	manager, err := newSboxctlServiceManager(options)
	if err != nil {
		return err
	}
	switch action {
	case "install":
		binary, err := trafficExecutablePath()
		if err != nil {
			return fmt.Errorf("resolve sboxctl path: %w", err)
		}
		if err := manager.InstallTrafficTimers(cmd.Context(), set.BaseDir, set.Global.Paths.Traffic, set.Global.Paths.Logs, binary); err != nil {
			return err
		}
		if _, err := manager.RunTrafficTimers(cmd.Context(), "enable", false); err != nil {
			return err
		}
		return writeStatus(cmd, outputStatusOK, "Traffic timers installed and enabled.")
	case "uninstall":
		if err := manager.UninstallTrafficTimers(cmd.Context()); err != nil {
			return err
		}
		return writeStatus(cmd, outputStatusOK, "Traffic timers uninstalled.")
	default:
		follow, _ := cmd.Flags().GetBool("follow")
		results, err := manager.RunTrafficTimers(cmd.Context(), action, follow)
		if err != nil {
			return err
		}
		return writeServiceResults(cmd, action, results)
	}
}

type trafficCommandContext struct {
	set      *config.AgentConfigSet
	options  traffic.Options
	location *time.Location
	repo     *traffic.Repository
}

// loadTrafficCommandContext 加载 traffic CLI 所需配置，并按需打开 DB。
func loadTrafficCommandContext(cmd *cobra.Command, openDB bool, readOnlyDB ...bool) (*trafficCommandContext, func(), error) {
	options, err := getRootOptions(cmd)
	if err != nil {
		return nil, nil, err
	}
	set, err := config.LoadAgentConfigSet(options.baseDir)
	if err != nil {
		return nil, nil, err
	}
	trafficOptions, err := effectiveTrafficOptions(cmd, set.Global)
	if err != nil {
		return nil, nil, err
	}
	location, err := traffic.LoadLocation(trafficOptions.Timezone)
	if err != nil {
		return nil, nil, err
	}
	ctx := &trafficCommandContext{set: set, options: trafficOptions, location: location}
	closeRepo := func() {}
	if openDB {
		readOnly := len(readOnlyDB) > 0 && readOnlyDB[0]
		var repo *traffic.Repository
		if readOnly {
			repo, err = traffic.OpenRepositoryReadOnly(trafficOptions.DBPath)
		} else {
			repo, err = traffic.OpenRepository(trafficOptions.DBPath)
		}
		if err != nil {
			return nil, nil, err
		}
		ctx.repo = repo
		closeRepo = func() {
			_ = repo.Close()
		}
	}
	return ctx, closeRepo, nil
}

// effectiveTrafficOptions 合并全局默认、独立配置文件和 CLI 覆盖。
func effectiveTrafficOptions(cmd *cobra.Command, global domain.GlobalConfig) (traffic.Options, error) {
	options := traffic.OptionsFromGlobal(global)
	options.DBPath = filepath.Join(global.Paths.Traffic, "traffic.db")
	path := trafficConfigPath(global)
	if _, err := os.Stat(path); err == nil {
		configValue, err := config.LoadTrafficConfig(path)
		if err != nil {
			return options, err
		}
		options = traffic.ApplyTrafficConfig(options, *configValue)
	} else if err != nil && !os.IsNotExist(err) {
		return options, fmt.Errorf("read traffic config %s: %w", path, err)
	}
	if value, _ := cmd.Flags().GetString("db"); value != "" {
		options.DBPath = value
	}
	if value, _ := cmd.Flags().GetString("timezone"); value != "" {
		options.Timezone = value
	}
	if value, _ := cmd.Flags().GetInt("retention-days"); value > 0 {
		options.RetentionDays = value
	}
	if value, _ := cmd.Flags().GetInt("timeout"); value > 0 {
		options.Timeout = time.Duration(value) * time.Second
	}
	return options, nil
}

// trafficTargetsAndFilter 解析 instance/scope/name/limit 查询参数。
func trafficTargetsAndFilter(cmd *cobra.Command, ctx *trafficCommandContext) ([]traffic.Target, traffic.Filter, error) {
	instance := instanceFlagValue(cmd)
	scope, _ := cmd.Flags().GetString("scope")
	name, _ := cmd.Flags().GetString("name")
	limit, _ := cmd.Flags().GetInt("limit")
	if err := traffic.ValidateScope(scope); err != nil {
		return nil, traffic.Filter{}, err
	}
	targets, err := traffic.SelectTargets(ctx.set.Instances, instance)
	if err != nil {
		return nil, traffic.Filter{}, err
	}
	filter := traffic.Filter{
		Scope: scope,
		Name:  name,
		Limit: limit,
	}
	return targets, filter, nil
}

// trafficInstanceNamesAndFilter 解析历史查询所需实例名和过滤条件。
func trafficInstanceNamesAndFilter(cmd *cobra.Command, ctx *trafficCommandContext) ([]string, traffic.Filter, error) {
	instance := instanceFlagValue(cmd)
	scope, _ := cmd.Flags().GetString("scope")
	name, _ := cmd.Flags().GetString("name")
	limit, _ := cmd.Flags().GetInt("limit")
	if err := traffic.ValidateScope(scope); err != nil {
		return nil, traffic.Filter{}, err
	}
	names, err := selectTrafficInstanceNames(ctx.set.Instances, instance)
	if err != nil {
		return nil, traffic.Filter{}, err
	}
	filter := traffic.Filter{
		Instances: names,
		Scope:     scope,
		Name:      name,
		Limit:     limit,
	}
	return names, filter, nil
}

// selectTrafficInstanceNames 为历史查询选择实例名，不要求 stats API 可用。
func selectTrafficInstanceNames(instances []domain.Instance, target string) ([]string, error) {
	if strings.TrimSpace(target) == "" {
		return nil, fmt.Errorf("--instance NAME|ALL is required")
	}
	names := []string{}
	for _, instance := range instances {
		domain.ApplyInstanceDefaults(&instance)
		if target == traffic.AllInstancesName {
			if instance.Enabled && instance.Traffic.Enabled {
				names = append(names, instance.Name)
			}
			continue
		}
		if instance.Name == target {
			return []string{instance.Name}, nil
		}
	}
	if target == traffic.AllInstancesName {
		if len(names) == 0 {
			return nil, fmt.Errorf("no queryable traffic instances")
		}
		return names, nil
	}
	return nil, fmt.Errorf("instance %q does not exist", target)
}

// instanceFlagValue 读取 --instance 参数。
func instanceFlagValue(cmd *cobra.Command) string {
	instance, _ := cmd.Flags().GetString("instance")
	return instance
}

// rangeOptionsFromCommand 读取时间范围相关 CLI 参数。
func rangeOptionsFromCommand(cmd *cobra.Command) traffic.RangeOptions {
	options := traffic.RangeOptions{}
	options.Date, _ = cmd.Flags().GetString("date")
	options.From, _ = cmd.Flags().GetString("from")
	options.To, _ = cmd.Flags().GetString("to")
	options.Days, _ = cmd.Flags().GetInt("days")
	options.Month, _ = cmd.Flags().GetString("month")
	options.Months, _ = cmd.Flags().GetInt("months")
	yearText, _ := cmd.Flags().GetString("year")
	options.Year = yearText
	options.Years, _ = cmd.Flags().GetInt("years")
	return options
}

// shouldAppendSubtotal 判断是否需要追加 ALL 小计。
func shouldAppendSubtotal(cmd *cobra.Command) bool {
	instance, _ := cmd.Flags().GetString("instance")
	return instance == traffic.AllInstancesName
}

// currentAsPeriod 将 current 增量转换为指定展示周期。
func currentAsPeriod(records []traffic.Record, period string, now time.Time, location *time.Location) []traffic.Record {
	switch period {
	case traffic.PeriodHourly:
		window := traffic.HourRange(now, location)
		converted := make([]traffic.Record, 0, len(records))
		for _, record := range records {
			record.Period = traffic.PeriodHourly
			record.StartTS = window.Start.Unix()
			record.EndTS = window.End.Unix()
			record.StartTime = traffic.FormatTime(window.Start, location)
			record.EndTime = traffic.FormatTime(window.End, location)
			converted = append(converted, record)
		}
		return converted
	case traffic.PeriodDaily:
		return traffic.AggregateRecords(records, traffic.PeriodDaily, traffic.DayRange(now, location), now.UTC(), location)
	case traffic.PeriodMonthly:
		return traffic.AggregateRecords(records, traffic.PeriodMonthly, traffic.MonthRange(now, location), now.UTC(), location)
	case traffic.PeriodYearly:
		return traffic.AggregateRecords(records, traffic.PeriodYearly, traffic.YearRange(now.In(location).Year(), location), now.UTC(), location)
	default:
		return nil
	}
}

// trafficConfigPath 返回独立 traffic 配置路径。
func trafficConfigPath(global domain.GlobalConfig) string {
	return filepath.Join(global.Paths.Traffic, "config.yaml")
}

// trafficConfigFromDefaults 将全局 traffic defaults 转成独立配置结构。
func trafficConfigFromDefaults(defaults domain.TrafficDefaultsConfig) domain.TrafficConfig {
	return domain.TrafficConfig{
		Version:                1,
		Enabled:                defaults.Enabled,
		Timezone:               defaults.Timezone,
		RetentionDays:          defaults.RetentionDays,
		DailyMinRetentionDays:  defaults.DailyMinRetentionDays,
		MonthlyRetentionMonths: defaults.MonthlyRetentionMonths,
		TimeoutSeconds:         defaults.TimeoutSeconds,
		Timer:                  defaults.Timer,
	}
}
