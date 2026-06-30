package cli

import "github.com/spf13/cobra"

const sboxsubDefaultBaseDir = "/opt/sbox-sub"

const (
	sboxsubGroupConfig      = "config"
	sboxsubGroupInput       = "input"
	sboxsubGroupHTTP        = "http"
	sboxsubGroupService     = "service"
	sboxsubGroupDiagnostics = "diagnostics"
)

// newSboxsubCommand 创建 sboxsub 的根命令树。
func newSboxsubCommand() *cobra.Command {
	root := newRootCommand(
		"sboxsub",
		sboxsubDefaultBaseDir,
		"Run and manage the subscription service",
		"sboxsub manages only subscription service configuration, input bundles, HTTP service, and subscription service lifecycle; it does not read agent configuration or runtime.",
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

	addSboxsubCommandGroups(root)
	addSboxsubFlags(root)
	localizeCommandHelp(root)
	return root
}

// addSboxsubCommandGroups 设置 sboxsub 根命令 usage 的功能分组。
func addSboxsubCommandGroups(root *cobra.Command) {
	addCommandGroups(root,
		&cobra.Group{ID: sboxsubGroupConfig, Title: "Setup and Configuration"},
		&cobra.Group{ID: sboxsubGroupInput, Title: "Inputs"},
		&cobra.Group{ID: sboxsubGroupHTTP, Title: "HTTP Service"},
		&cobra.Group{ID: sboxsubGroupService, Title: "Runtime and Service Files"},
		&cobra.Group{ID: sboxsubGroupDiagnostics, Title: "Diagnostics"},
		&cobra.Group{ID: commandGroupHelp, Title: "Help"},
	)
	setCommandGroup(root, sboxsubGroupConfig, "init", "config", "clear", "version")
	setCommandGroup(root, sboxsubGroupInput, "import", "input")
	setCommandGroup(root, sboxsubGroupHTTP, "serve")
	setCommandGroup(root, sboxsubGroupService, "start", "stop", "restart", "status", "logs", "enable", "disable", "service")
	setCommandGroup(root, sboxsubGroupDiagnostics, "doctor")
}

// addSboxsubFlags 为 sboxsub 占位命令补充规格中的常用参数。
func addSboxsubFlags(root *cobra.Command) {
	mustCommand(root, "init").Flags().Bool("force", false, "overwrite existing initialization result")
	mustCommand(root, "config").Flags().String("editor", "", "editor command")
	mustCommand(root, "config", "show").Flags().Bool("show-secrets", false, "show sensitive fields in plaintext")
	mustCommand(root, "import").Flags().Bool("replace-all", false, "replace all existing inputs")
	mustCommand(root, "logs").Flags().BoolP("follow", "f", false, "follow logs")
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
