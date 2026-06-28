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
		newSboxsubInitCommandT05(),
		newVersionCommand("version", false),
		newSboxsubConfigCommand(),
		newSboxsubImportCommandT05(),
		newSboxsubClearCommandT05(),
		newSboxsubInputCommand(),
		newSboxsubServeCommand(),
		newSboxsubServiceCommand(),
		newSboxsubServiceActionCommand("start"),
		newSboxsubServiceActionCommand("stop"),
		newSboxsubServiceActionCommand("restart"),
		newSboxsubServiceActionCommand("status"),
		newSboxsubServiceActionCommand("logs"),
		newSboxsubServiceActionCommand("enable"),
		newSboxsubServiceActionCommand("disable"),
		newSboxsubDoctorCommandT05(),
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
	return newSboxsubConfigCommandT05()
}

// newSboxsubInputCommand 创建订阅 input 管理命令组。
func newSboxsubInputCommand() *cobra.Command {
	return newSboxsubInputCommandT05()
}

// newSboxsubServeCommand 创建订阅 HTTP 服务命令。
func newSboxsubServeCommand() *cobra.Command {
	return newSboxsubServeCommandT05()
}

// newSboxsubServiceCommand 创建订阅服务文件命令组。
func newSboxsubServiceCommand() *cobra.Command {
	return newSboxsubServiceCommandT05()
}
