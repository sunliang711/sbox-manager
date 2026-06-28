package cli

import "github.com/spf13/cobra"

const sboxctlDefaultBaseDir = "/opt/sbox-manager"

// newSboxctlCommand 创建 sboxctl 的根命令树。
func newSboxctlCommand() *cobra.Command {
	root := newRootCommand(
		"sboxctl",
		sboxctlDefaultBaseDir,
		"管理 sing-box agent 实例",
		"sboxctl 管理本机 sing-box agent 配置、实例生命周期、安装、订阅导出、流量统计、诊断和备份。",
		false,
	)

	root.AddCommand(
		newSboxctlInitCommand(),
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
		newStubCommand("export", "导出 agent 配置备份"),
		newStubCommand("import BACKUP", "导入 agent 配置备份"),
		newStubCommand("doctor", "执行 agent 诊断检查"),
		newStubCommand("ipinfo INSTANCE", "查询实例 IP 信息"),
		newSboxctlTrafficCommand(),
	)

	addSboxctlFlags(root)
	return root
}

// addSboxctlFlags 为 sboxctl 占位命令补充规格中的常用参数。
func addSboxctlFlags(root *cobra.Command) {
	mustCommand(root, "init").Flags().String("external-host", "", "外部访问域名或地址")
	mustCommand(root, "init").Flags().Bool("force", false, "覆盖已有初始化结果")

	setup := mustCommand(root, "setup")
	setup.Flags().String("external-host", "", "外部访问域名或地址")
	setup.Flags().Bool("force", false, "覆盖已有初始化结果")
	setup.Flags().Bool("start", false, "安装后启动服务")

	config := mustCommand(root, "config")
	config.Flags().String("editor", "", "指定编辑器命令")
	config.Flags().Bool("check-only", false, "只检查配置不写入")

	add := mustCommand(root, "add")
	add.Flags().String("template", "edge", "实例模板：edge、relay、urltest")
	add.Flags().String("from-file", "", "从指定文件创建实例")
	add.Flags().Bool("allocate-ports", true, "自动分配端口")
	add.Flags().Bool("keep-template-ports", false, "保留模板端口")
	add.Flags().Bool("edit", true, "创建后打开编辑器")
	add.Flags().Bool("no-edit", false, "创建后不打开编辑器")
	add.Flags().String("editor", "", "指定编辑器命令")

	list := mustCommand(root, "list")
	list.Flags().Bool("verbose", false, "输出详细信息")
	list.Flags().Bool("check-system-ports", false, "检查系统端口占用")

	clone := mustCommand(root, "clone")
	clone.Flags().Bool("allocate-ports", true, "自动分配端口")
	clone.Flags().Bool("edit", true, "克隆后打开编辑器")
	clone.Flags().Bool("no-edit", false, "克隆后不打开编辑器")
	clone.Flags().String("editor", "", "指定编辑器命令")

	mustCommand(root, "remove").Flags().Bool("purge", false, "清理关联生成物")
	mustCommand(root, "logs").Flags().BoolP("follow", "f", false, "持续跟随日志")
	mustCommand(root, "sub", "export").Flags().StringP("output", "o", "", "输出文件")
	mustCommand(root, "sub", "export").Flags().Bool("summary", false, "只输出摘要")
	mustCommand(root, "sub", "export").Flags().Bool("dry-run", false, "预览导出结果")
	mustCommand(root, "sub", "validate-inputs").Flags().String("input-dir", "", "订阅 input 目录")
	mustCommand(root, "export").Flags().StringP("output", "o", "", "输出文件")
	mustCommand(root, "import").Flags().Bool("force", false, "强制导入")
	mustCommand(root, "ipinfo").Flags().String("family", "all", "地址族：all、ipv4、ipv6")
	mustCommand(root, "ipinfo").Flags().Int("timeout", 0, "超时时间，单位秒")
}

// newSboxctlInstallCommand 创建 sing-box 和规则资源安装命令。
func newSboxctlInstallCommand() *cobra.Command {
	return newResourceCommand("install", "安装 sing-box、规则资源或全部资源")
}

// newSboxctlUpdateCommand 创建 sing-box 和规则资源更新命令。
func newSboxctlUpdateCommand() *cobra.Command {
	return newResourceCommand("update", "更新 sing-box、规则资源或全部资源")
}

// newSboxctlUninstallCommand 创建 sing-box 和规则资源卸载命令。
func newSboxctlUninstallCommand() *cobra.Command {
	cmd := newResourceCommand("uninstall", "卸载 sing-box、规则资源或全部资源")
	cmd.Flags().Bool("purge", false, "清理下载缓存和受管文件")
	return cmd
}

// newSboxctlSubCommand 创建 agent 侧订阅导出命令组。
func newSboxctlSubCommand() *cobra.Command {
	return newStubGroup(
		"sub",
		"导出和校验 agent 订阅输入",
		newStubCommand("export [INSTANCE]", "导出订阅 bundle"),
		newStubCommand("validate-inputs", "校验订阅 inputs"),
	)
}

// newSboxctlTrafficCommand 创建流量统计命令树。
func newSboxctlTrafficCommand() *cobra.Command {
	traffic := newStubGroup(
		"traffic",
		"采集、查询、导出和维护流量统计数据",
		newTrafficCollectCommand(),
		newTrafficShowCommand(),
		newTrafficWatchCommand(),
		newTrafficSummarizeCommand(),
		newTrafficExportCommand(),
		newStubGroup("list", "列出流量统计资源", newStubCommand("instances", "列出统计实例")),
		newStubGroup("cleanup", "清理历史流量记录", newStubCommand("records", "清理历史记录")),
		newStubGroup("check", "检查流量统计配置或健康状态", newStubCommand("config", "检查流量配置"), newStubCommand("health", "检查统计健康状态")),
		newStubGroup("edit", "编辑流量统计配置", newStubCommand("config", "编辑流量配置")),
		newTrafficTimerCommand(),
	)

	traffic.PersistentFlags().String("db", "", "流量统计 SQLite 文件")
	traffic.PersistentFlags().String("timezone", "", "统计时区")
	traffic.PersistentFlags().Int("retention-days", 0, "hourly/daily 保留天数")
	traffic.PersistentFlags().Int("timeout", 0, "请求超时时间，单位秒")
	mustCommand(traffic, "cleanup", "records").Flags().Int("monthly-retention-months", 0, "monthly 保留月数")
	mustCommand(traffic, "cleanup", "records").Flags().String("period", "all", "清理周期：hourly、daily、monthly、all")
	mustCommand(traffic, "cleanup", "records").Flags().Bool("dry-run", false, "只预览不删除")
	mustCommand(traffic, "edit", "config").Flags().String("editor", "", "指定编辑器命令")
	return traffic
}

// newTrafficCollectCommand 创建流量采集命令组。
func newTrafficCollectCommand() *cobra.Command {
	collect := newStubGroup(
		"collect",
		"采集周期流量数据",
		newStubCommand("hourly", "采集小时流量"),
		newStubCommand("daily", "聚合日流量"),
		newStubCommand("monthly", "聚合月流量"),
	)
	for _, name := range []string{"hourly", "daily", "monthly"} {
		child := mustCommand(collect, name)
		child.Flags().String("instance", "", "实例名称或 ALL")
	}
	mustCommand(collect, "hourly").Flags().String("at", "", "采集时间，RFC3339")
	mustCommand(collect, "daily").Flags().String("date", "", "统计日期，YYYY-MM-DD")
	mustCommand(collect, "monthly").Flags().String("month", "", "统计月份，YYYY-MM")
	return collect
}

// newTrafficShowCommand 创建流量查询命令组。
func newTrafficShowCommand() *cobra.Command {
	show := newStubGroup(
		"show",
		"查询流量统计数据",
		newStubCommand("current", "查询当前周期流量"),
		newStubCommand("hourly", "查询小时流量"),
		newStubCommand("daily", "查询日流量"),
		newStubCommand("monthly", "查询月流量"),
		newStubCommand("yearly", "查询年流量"),
	)
	for _, name := range []string{"current", "hourly", "daily", "monthly", "yearly"} {
		child := mustCommand(show, name)
		addTrafficQueryFlags(child)
	}
	mustCommand(show, "hourly").Flags().String("date", "", "统计日期，YYYY-MM-DD")
	mustCommand(show, "daily").Flags().String("date", "", "统计日期，YYYY-MM-DD")
	mustCommand(show, "monthly").Flags().String("month", "", "统计月份，YYYY-MM")
	mustCommand(show, "yearly").Flags().String("year", "", "统计年份，YYYY")
	return show
}

// newTrafficWatchCommand 创建流量 watch 命令组。
func newTrafficWatchCommand() *cobra.Command {
	watch := newStubGroup("watch", "持续观察流量数据", newStubCommand("current", "观察当前周期流量"))
	current := mustCommand(watch, "current")
	addTrafficQueryFlags(current)
	current.Flags().Int("interval", 0, "刷新间隔，单位秒")
	current.Flags().Int("count", 0, "刷新次数")
	current.Flags().Bool("no-clear", false, "刷新时不清屏")
	return watch
}

// newTrafficSummarizeCommand 创建流量汇总命令组。
func newTrafficSummarizeCommand() *cobra.Command {
	summarize := newStubGroup(
		"summarize",
		"汇总流量统计数据",
		newStubCommand("hourly", "汇总小时流量"),
		newStubCommand("daily", "汇总日流量"),
		newStubCommand("monthly", "汇总月流量"),
	)
	for _, name := range []string{"hourly", "daily", "monthly"} {
		child := mustCommand(summarize, name)
		addTrafficQueryFlags(child)
		child.Flags().String("date", "", "统计日期，YYYY-MM-DD")
	}
	mustCommand(summarize, "monthly").Flags().String("month", "", "统计月份，YYYY-MM")
	return summarize
}

// newTrafficExportCommand 创建流量导出命令组。
func newTrafficExportCommand() *cobra.Command {
	export := newStubGroup(
		"export",
		"导出流量统计数据",
		newStubCommand("hourly", "导出小时流量"),
		newStubCommand("daily", "导出日流量"),
		newStubCommand("monthly", "导出月流量"),
	)
	for _, name := range []string{"hourly", "daily", "monthly"} {
		child := mustCommand(export, name)
		addTrafficQueryFlags(child)
		child.Flags().String("date", "", "统计日期，YYYY-MM-DD")
		child.Flags().String("format", "csv", "导出格式")
		child.Flags().String("output", "", "输出文件")
	}
	return export
}

// newTrafficTimerCommand 创建流量统计调度器命令组。
func newTrafficTimerCommand() *cobra.Command {
	timer := newStubGroup(
		"timer",
		"管理流量统计调度器",
		newStubCommand("install", "安装调度器服务文件"),
		newStubCommand("uninstall", "卸载调度器服务文件"),
		newStubCommand("enable", "启用调度器"),
		newStubCommand("disable", "禁用调度器"),
		newStubCommand("status", "查看调度器状态"),
		newStubCommand("logs", "查看调度器日志"),
		newStubGroup("run", "立即运行一次统计任务", newStubCommand("hourly", "立即运行 hourly 任务"), newStubCommand("daily", "立即运行 daily 任务"), newStubCommand("monthly", "立即运行 monthly 任务")),
	)
	mustCommand(timer, "logs").Flags().BoolP("follow", "f", false, "持续跟随日志")
	return timer
}

// addTrafficQueryFlags 为流量查询类命令追加共享参数。
func addTrafficQueryFlags(cmd *cobra.Command) {
	cmd.Flags().String("instance", "", "实例名称或 ALL")
	cmd.Flags().String("scope", "", "统计维度：user、inbound、outbound")
	cmd.Flags().String("name", "", "维度名称")
	cmd.Flags().String("from", "", "起始日期，YYYY-MM-DD")
	cmd.Flags().String("to", "", "结束日期，YYYY-MM-DD")
	cmd.Flags().Int("days", 0, "最近天数")
	cmd.Flags().Int("months", 0, "最近月数")
	cmd.Flags().Int("years", 0, "最近年数")
	cmd.Flags().Int("limit", 0, "最大返回行数")
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
