package cli

import "github.com/spf13/cobra"

const sboxsubDefaultBaseDir = "/opt/sbox-sub"

// newSboxsubCommand 创建 sboxsub 的根命令树。
func newSboxsubCommand() *cobra.Command {
	root := newRootCommand(
		"sboxsub",
		sboxsubDefaultBaseDir,
		"运行和管理订阅服务",
		"sboxsub 只管理订阅服务自身配置、输入 bundle、HTTP 服务和订阅服务生命周期，不读取 agent 配置或 runtime。",
		true,
	)

	root.AddCommand(
		newStubCommand("init", "初始化订阅服务环境目录和默认配置"),
		newVersionCommand("version", false),
		newSboxsubConfigCommand(),
		newStubCommand("import BUNDLE", "导入订阅 bundle"),
		newStubCommand("clear", "清空订阅服务数据"),
		newSboxsubInputCommand(),
		newSboxsubServeCommand(),
		newSboxsubServiceCommand(),
		newStubCommand("start", "启动订阅服务"),
		newStubCommand("stop", "停止订阅服务"),
		newStubCommand("restart", "重启订阅服务"),
		newStubCommand("status", "查看订阅服务状态"),
		newStubCommand("logs", "查看订阅服务日志"),
		newStubCommand("enable", "启用订阅服务"),
		newStubCommand("disable", "禁用订阅服务"),
		newStubCommand("doctor", "执行订阅服务诊断检查"),
	)

	addSboxsubFlags(root)
	return root
}

// addSboxsubFlags 为 sboxsub 占位命令补充规格中的常用参数。
func addSboxsubFlags(root *cobra.Command) {
	mustCommand(root, "init").Flags().Bool("force", false, "覆盖已有初始化结果")
	mustCommand(root, "config").Flags().String("editor", "", "指定编辑器命令")
	mustCommand(root, "config", "show").Flags().Bool("show-secrets", false, "显示敏感字段明文")
	mustCommand(root, "import").Flags().Bool("replace-all", false, "替换所有现有输入")
	mustCommand(root, "logs").Flags().BoolP("follow", "f", false, "持续跟随日志")
}

// newSboxsubConfigCommand 创建订阅服务配置命令组。
func newSboxsubConfigCommand() *cobra.Command {
	return newStubGroup(
		"config",
		"编辑、展示或检查订阅服务配置",
		newStubCommand("show", "展示订阅服务配置"),
		newStubCommand("check", "检查订阅服务配置"),
	)
}

// newSboxsubInputCommand 创建订阅 input 管理命令组。
func newSboxsubInputCommand() *cobra.Command {
	input := newStubGroup(
		"input",
		"管理订阅服务输入源",
		newStubCommand("list", "列出输入源"),
		newStubCommand("show SOURCE", "展示输入源"),
		newStubCommand("validate [SOURCE]", "校验输入源"),
		newStubCommand("edit SOURCE", "编辑输入源"),
		newStubCommand("clone SOURCE TARGET", "克隆输入源"),
		newStubCommand("set-host HOST [SOURCE]", "设置输入源 external host"),
		newStubCommand("remove SOURCE", "移除输入源"),
	)
	mustCommand(input, "show").Flags().Bool("raw", false, "输出原始内容")
	mustCommand(input, "show").Flags().Bool("show-secrets", false, "显示敏感字段明文")
	mustCommand(input, "edit").Flags().String("editor", "", "指定编辑器命令")
	mustCommand(input, "clone").Flags().String("editor", "", "指定编辑器命令")
	mustCommand(input, "set-host").Flags().Bool("all", false, "应用到全部输入源")
	return input
}

// newSboxsubServeCommand 创建订阅 HTTP 服务命令。
func newSboxsubServeCommand() *cobra.Command {
	serve := newStubCommand("serve", "启动订阅 HTTP 服务")
	serve.Flags().String("host", "", "覆盖 HTTP 监听 host")
	serve.Flags().Int("port", 0, "覆盖 HTTP 监听 port")
	return serve
}

// newSboxsubServiceCommand 创建订阅服务文件命令组。
func newSboxsubServiceCommand() *cobra.Command {
	return newStubGroup(
		"service",
		"管理订阅服务 systemd unit 或 launchd plist",
		newStubCommand("install", "安装订阅服务文件"),
		newStubCommand("uninstall", "卸载订阅服务文件"),
	)
}
