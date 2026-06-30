package cli

import "github.com/spf13/cobra"

const sboxctlDefaultBaseDir = "/opt/sbox-manager"

const (
	sboxctlGroupConfig      = "config"
	sboxctlGroupInstance    = "instance"
	sboxctlGroupRender      = "render"
	sboxctlGroupService     = "service"
	sboxctlGroupResource    = "resource"
	sboxctlGroupTraffic     = "traffic"
	sboxctlGroupDiagnostics = "diagnostics"
)

// newSboxctlCommand 创建 sboxctl 的根命令树。
func newSboxctlCommand() *cobra.Command {
	root := newRootCommand(
		"sboxctl",
		sboxctlDefaultBaseDir,
		"Manage sing-box agent instances",
		"sboxctl manages local sing-box agent configuration, instance lifecycle, installation, subscription export, traffic statistics, diagnostics, and backups.",
		false,
	)

	root.AddCommand(
		newSboxctlSetupCommand(),
		newSboxctlConfigCommand(),
		newSboxctlExampleCommand(),
		newSboxctlAddCommand(),
		newSboxctlListCommand(),
		newSboxctlCloneCommand(),
		newSboxctlMemberCommand(),
		newSboxctlRemoveCommand(),
		newSboxctlValidateCommand(),
		newSboxctlCheckCommand(),
		newSboxctlRenderCommand(),
		newSboxctlExportConfigCommand(),
		newSboxctlLifecycleCommand("start"),
		newSboxctlServiceActionCommand("stop"),
		newSboxctlLifecycleCommand("restart"),
		newSboxctlServiceActionCommand("status"),
		newSboxctlServiceActionCommand("logs"),
		newSboxctlServiceActionCommand("enable"),
		newSboxctlServiceActionCommand("disable"),
		newSboxctlServiceCommand(),
		newSboxctlInstallCommand(),
		newSboxctlUpdateCommand(),
		newSboxctlUninstallCommand(),
		newVersionCommand("version [sing-box|rules]", true),
		newSboxctlSubCommand(),
		newSboxctlExportCommandT07(),
		newSboxctlImportCommandT07(),
		newSboxctlDoctorCommandT07(),
		newSboxctlIPInfoCommandT07(),
		newSboxctlTrafficCommand(),
	)

	addSboxctlCommandGroups(root)
	addSboxctlFlags(root)
	localizeCommandHelp(root)
	return root
}

// addSboxctlCommandGroups 设置 sboxctl 根命令 usage 的功能分组。
func addSboxctlCommandGroups(root *cobra.Command) {
	addCommandGroups(root,
		&cobra.Group{ID: sboxctlGroupConfig, Title: "Setup and Configuration"},
		&cobra.Group{ID: sboxctlGroupInstance, Title: "Instances"},
		&cobra.Group{ID: sboxctlGroupRender, Title: "Render and Subscription"},
		&cobra.Group{ID: sboxctlGroupService, Title: "Runtime and Services"},
		&cobra.Group{ID: sboxctlGroupResource, Title: "Resources"},
		&cobra.Group{ID: sboxctlGroupTraffic, Title: "Traffic Statistics"},
		&cobra.Group{ID: sboxctlGroupDiagnostics, Title: "Diagnostics and Backups"},
		&cobra.Group{ID: commandGroupHelp, Title: "Help"},
	)
	setCommandGroup(root, sboxctlGroupConfig, "setup", "config", "example", "validate")
	setCommandGroup(root, sboxctlGroupInstance, "add", "list", "clone", "member", "remove")
	setCommandGroup(root, sboxctlGroupRender, "check", "render", "export-config", "sub")
	setCommandGroup(root, sboxctlGroupService, "start", "stop", "restart", "status", "logs", "enable", "disable", "service")
	setCommandGroup(root, sboxctlGroupResource, "install", "update", "uninstall", "version")
	setCommandGroup(root, sboxctlGroupTraffic, "traffic")
	setCommandGroup(root, sboxctlGroupDiagnostics, "doctor", "ipinfo", "export", "import")
}

// addSboxctlFlags 为 sboxctl 占位命令补充规格中的常用参数。
func addSboxctlFlags(root *cobra.Command) {
	setup := mustCommand(root, "setup")
	addSboxctlSetupLocalFlags(setup)
	addSboxctlSetupLocalFlags(mustCommand(root, "setup", "local"))
	addSboxctlSetupLocalFlags(mustCommand(root, "setup", "all"))

	config := mustCommand(root, "config")
	config.Flags().String("editor", "", "editor command")
	config.Flags().Bool("check-only", false, "check configuration without writing changes")

	add := mustCommand(root, "add")
	add.Flags().String("template", "edge", "instance template: edge, relay, urltest")
	add.Flags().String("from-file", "", "create instance from file")
	add.Flags().String("members", "", "urltest template member instances, comma-separated")
	add.Flags().Bool("allocate-ports", true, "allocate ports automatically")
	add.Flags().Bool("keep-template-ports", false, "keep template ports")
	add.Flags().Bool("edit", true, "open editor after creation")
	add.Flags().Bool("no-edit", false, "do not open editor after creation")
	add.Flags().String("editor", "", "editor command")

	list := mustCommand(root, "list")
	list.Flags().Bool("verbose", false, "print detailed information")
	list.Flags().Bool("check-system-ports", false, "check system port usage")

	clone := mustCommand(root, "clone")
	clone.Flags().Bool("allocate-ports", true, "allocate ports automatically")
	clone.Flags().Bool("edit", true, "open editor after cloning")
	clone.Flags().Bool("no-edit", false, "do not open editor after cloning")
	clone.Flags().String("editor", "", "editor command")

	mustCommand(root, "remove").Flags().Bool("purge", false, "clean related generated artifacts")
	mustCommand(root, "logs").Flags().BoolP("follow", "f", false, "follow logs")
	mustCommand(root, "sub", "export").Flags().StringP("output", "o", "", "output file")
	mustCommand(root, "sub", "export").Flags().Bool("summary", false, "print summary only")
	mustCommand(root, "sub", "export").Flags().Bool("dry-run", false, "preview export result")
	mustCommand(root, "sub", "validate-inputs").Flags().String("input-dir", "", "subscription input directory")
	mustCommand(root, "export").Flags().StringP("output", "o", "", "output file")
	mustCommand(root, "import").Flags().Bool("force", false, "force import")
	mustCommand(root, "ipinfo").Flags().String("family", "all", "address family: all, ipv4, ipv6")
	mustCommand(root, "ipinfo").Flags().Int("timeout", 0, "timeout in seconds")
}

// addSboxctlSetupLocalFlags 为会创建或覆盖本机配置的 setup 命令添加参数。
func addSboxctlSetupLocalFlags(cmd *cobra.Command) {
	cmd.Flags().String("external-host", "", "external access hostname or address")
	cmd.Flags().Bool("force", false, "overwrite existing initialization result")
}

// newSboxctlInstallCommand 创建 sing-box 和规则资源安装命令。
func newSboxctlInstallCommand() *cobra.Command {
	return newResourceCommand("install", "Install sing-box, rule resources, or all resources")
}

// newSboxctlUpdateCommand 创建 sing-box 和规则资源更新命令。
func newSboxctlUpdateCommand() *cobra.Command {
	return newResourceCommand("update", "Update sing-box, rule resources, or all resources")
}

// newSboxctlUninstallCommand 创建 sing-box 和规则资源卸载命令。
func newSboxctlUninstallCommand() *cobra.Command {
	cmd := newResourceCommand("uninstall", "Uninstall sing-box, rule resources, or all resources")
	cmd.Flags().Bool("purge", false, "remove download cache and managed files")
	return cmd
}

// newSboxctlSubCommand 创建 agent 侧订阅导出命令组。
func newSboxctlSubCommand() *cobra.Command {
	return newSboxctlSubCommandT05()
}

// newSboxctlTrafficCommand 创建流量统计命令树。
func newSboxctlTrafficCommand() *cobra.Command {
	return newSboxctlTrafficCommandT06()
}

// newSboxctlTrafficCommandStub 创建 T01/T05 使用的流量统计占位命令树。
func newSboxctlTrafficCommandStub() *cobra.Command {
	traffic := newStubGroup(
		"traffic",
		"Collect, query, export, and maintain traffic statistics",
		newTrafficCollectCommand(),
		newTrafficShowCommand(),
		newTrafficWatchCommand(),
		newTrafficSummarizeCommand(),
		newTrafficExportCommand(),
		newStubGroup("list", "List traffic statistic resources", newStubCommand("instances", "List statistic instances")),
		newStubGroup("cleanup", "Clean historical traffic records", newStubCommand("records", "Clean historical records")),
		newStubGroup("check", "Check traffic statistic configuration or health", newStubCommand("config", "Check traffic configuration"), newStubCommand("health", "Check statistic health")),
		newStubGroup("edit", "Edit traffic statistic configuration", newStubCommand("config", "Edit traffic configuration")),
		newTrafficTimerCommand(),
	)

	traffic.PersistentFlags().String("db", "", "traffic statistics SQLite file")
	traffic.PersistentFlags().String("timezone", "", "statistics timezone")
	traffic.PersistentFlags().Int("retention-days", 0, "hourly/daily retention days")
	traffic.PersistentFlags().Int("timeout", 0, "request timeout in seconds")
	mustCommand(traffic, "cleanup", "records").Flags().Int("monthly-retention-months", 0, "monthly retention months")
	mustCommand(traffic, "cleanup", "records").Flags().String("period", "all", "cleanup period: hourly, daily, monthly, all")
	mustCommand(traffic, "cleanup", "records").Flags().Bool("dry-run", false, "preview without deleting")
	mustCommand(traffic, "edit", "config").Flags().String("editor", "", "editor command")
	return traffic
}

// newTrafficCollectCommand 创建流量采集命令组。
func newTrafficCollectCommand() *cobra.Command {
	collect := newStubGroup(
		"collect",
		"Collect periodic traffic data",
		newStubCommand("hourly", "Collect hourly traffic"),
		newStubCommand("daily", "Aggregate daily traffic"),
		newStubCommand("monthly", "Aggregate monthly traffic"),
	)
	for _, name := range []string{"hourly", "daily", "monthly"} {
		child := mustCommand(collect, name)
		child.Flags().String("instance", "", "instance name or ALL")
	}
	mustCommand(collect, "hourly").Flags().String("at", "", "collection time, RFC3339")
	mustCommand(collect, "daily").Flags().String("date", "", "statistics date, YYYY-MM-DD")
	mustCommand(collect, "monthly").Flags().String("month", "", "statistics month, YYYY-MM")
	return collect
}

// newTrafficShowCommand 创建流量查询命令组。
func newTrafficShowCommand() *cobra.Command {
	show := newStubGroup(
		"show",
		"Query traffic statistics",
		newStubCommand("current", "Query current-period traffic"),
		newStubCommand("hourly", "Query hourly traffic"),
		newStubCommand("daily", "Query daily traffic"),
		newStubCommand("monthly", "Query monthly traffic"),
		newStubCommand("yearly", "Query yearly traffic"),
	)
	for _, name := range []string{"current", "hourly", "daily", "monthly", "yearly"} {
		child := mustCommand(show, name)
		addTrafficQueryFlags(child)
	}
	mustCommand(show, "hourly").Flags().String("date", "", "statistics date, YYYY-MM-DD")
	mustCommand(show, "daily").Flags().String("date", "", "statistics date, YYYY-MM-DD")
	mustCommand(show, "monthly").Flags().String("month", "", "statistics month, YYYY-MM")
	mustCommand(show, "yearly").Flags().String("year", "", "statistics year, YYYY")
	return show
}

// newTrafficWatchCommand 创建流量 watch 命令组。
func newTrafficWatchCommand() *cobra.Command {
	watch := newStubGroup("watch", "Watch traffic data continuously", newStubCommand("current", "Watch current-period traffic"))
	current := mustCommand(watch, "current")
	addTrafficQueryFlags(current)
	current.Flags().Int("interval", 0, "refresh interval in seconds")
	current.Flags().Int("count", 0, "refresh count")
	current.Flags().Bool("no-clear", false, "do not clear the screen on refresh")
	return watch
}

// newTrafficSummarizeCommand 创建流量汇总命令组。
func newTrafficSummarizeCommand() *cobra.Command {
	summarize := newStubGroup(
		"summarize",
		"Summarize traffic statistics",
		newStubCommand("hourly", "Summarize hourly traffic"),
		newStubCommand("daily", "Summarize daily traffic"),
		newStubCommand("monthly", "Summarize monthly traffic"),
	)
	for _, name := range []string{"hourly", "daily", "monthly"} {
		child := mustCommand(summarize, name)
		addTrafficQueryFlags(child)
		child.Flags().String("date", "", "statistics date, YYYY-MM-DD")
	}
	mustCommand(summarize, "monthly").Flags().String("month", "", "statistics month, YYYY-MM")
	return summarize
}

// newTrafficExportCommand 创建流量导出命令组。
func newTrafficExportCommand() *cobra.Command {
	export := newStubGroup(
		"export",
		"Export traffic statistics",
		newStubCommand("hourly", "Export hourly traffic"),
		newStubCommand("daily", "Export daily traffic"),
		newStubCommand("monthly", "Export monthly traffic"),
	)
	for _, name := range []string{"hourly", "daily", "monthly"} {
		child := mustCommand(export, name)
		addTrafficQueryFlags(child)
		child.Flags().String("date", "", "statistics date, YYYY-MM-DD")
		child.Flags().String("format", "csv", "export format")
		child.Flags().String("output", "", "output file")
	}
	return export
}

// newTrafficTimerCommand 创建流量统计调度器命令组。
func newTrafficTimerCommand() *cobra.Command {
	timer := newStubGroup(
		"timer",
		"Manage the traffic statistics scheduler",
		newStubCommand("install", "Install scheduler service files"),
		newStubCommand("uninstall", "Uninstall scheduler service files"),
		newStubCommand("enable", "Enable scheduler"),
		newStubCommand("disable", "Disable scheduler"),
		newStubCommand("status", "Show scheduler status"),
		newStubCommand("logs", "Show scheduler logs"),
		newStubGroup("run", "Run a statistics task immediately", newStubCommand("hourly", "Run the hourly task immediately"), newStubCommand("daily", "Run the daily task immediately"), newStubCommand("monthly", "Run the monthly task immediately")),
	)
	mustCommand(timer, "logs").Flags().BoolP("follow", "f", false, "follow logs")
	return timer
}

// addTrafficQueryFlags 为流量查询类命令追加共享参数。
func addTrafficQueryFlags(cmd *cobra.Command) {
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

// mustCommand 按路径查找已注册命令，路径错误表示命令树定义自身有问题。
func mustCommand(root *cobra.Command, names ...string) *cobra.Command {
	current := root
	for _, name := range names {
		next := findCommand(current, name)
		if next == nil {
			panic("missing command: " + name)
		}
		current = next
	}
	return current
}

// findCommand 在直接子命令中按首个 use 片段查找命令。
func findCommand(root *cobra.Command, name string) *cobra.Command {
	for _, child := range root.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}
