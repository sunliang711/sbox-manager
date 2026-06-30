package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/version"
)

const (
	defaultServiceManager = "auto"
	serviceManagerSystemd = "systemd"
	serviceManagerLaunchd = "launchd"
)

// ErrNotImplemented 表示命令树中已占位但尚未实现的命令。
var ErrNotImplemented = errors.New("not implemented")

type rootOptions struct {
	baseDir        string
	serviceManager string
	listen         string
	logger         zerolog.Logger
}

type rootOptionsContextKey struct{}

const (
	commandGroupHelp = "help"
)

const localizedHelpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{.UsageString}}`

const localizedUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Other Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Options:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Options:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional Help Topics:
{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Run "{{.CommandPath}} [command] --help" for command-specific help.{{end}}
`

// RunSboxctl 执行 sboxctl 命令入口，适用于 cmd/sboxctl/main.go。
func RunSboxctl() {
	runAndExit(newSboxctlCommand())
}

// RunSboxsub 执行 sboxsub 命令入口，适用于 cmd/sboxsub/main.go。
func RunSboxsub() {
	runAndExit(newSboxsubCommand())
}

// runAndExit 执行根命令，并在失败时输出用户可读的错误摘要。
func runAndExit(cmd *cobra.Command) {
	if err := cmd.Execute(); err != nil {
		if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err); writeErr != nil {
			os.Exit(1)
		}
		os.Exit(1)
	}
}

// newRootCommand 创建带通用全局参数和基础日志上下文的根命令。
func newRootCommand(name string, defaultBaseDir string, short string, long string, includeListen bool) *cobra.Command {
	opts := &rootOptions{
		baseDir:        defaultBaseDir,
		serviceManager: defaultServiceManager,
	}

	root := &cobra.Command{
		Use:               name,
		Short:             short,
		Long:              long,
		SilenceUsage:      true,
		SilenceErrors:     true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := validateServiceManager(opts.serviceManager); err != nil {
				return err
			}
			if includeListen {
				if err := validateListenAddress(opts.listen); err != nil {
					return err
				}
			}

			// T01 只接入基础日志上下文，不在占位命令中产生业务日志或副作用。
			opts.logger = newLogger(cmd.ErrOrStderr(), name)
			ctx := opts.logger.WithContext(cmd.Context())
			cmd.SetContext(context.WithValue(ctx, rootOptionsContextKey{}, opts))
			return nil
		},
	}

	root.PersistentFlags().StringVar(&opts.baseDir, "base-dir", defaultBaseDir, "environment directory")
	root.PersistentFlags().StringVar(&opts.serviceManager, "service-manager", defaultServiceManager, "service manager: auto, systemd, launchd")
	if includeListen {
		root.PersistentFlags().StringVar(&opts.listen, "listen", "", "override subscription HTTP listen address, format HOST:PORT")
	}

	return root
}

// getRootOptions 从命令上下文读取全局参数。
func getRootOptions(cmd *cobra.Command) (*rootOptions, error) {
	options, ok := cmd.Context().Value(rootOptionsContextKey{}).(*rootOptions)
	if !ok || options == nil {
		return nil, fmt.Errorf("failed to read CLI global options")
	}
	return options, nil
}

// addCommandGroups 为父命令注册 usage 中展示的功能分组。
func addCommandGroups(parent *cobra.Command, groups ...*cobra.Group) {
	parent.AddGroup(groups...)
	parent.SetHelpCommandGroupID(commandGroupHelp)
}

// localizeCommandBasics 统一单个命令的 help 模板和 -h 文案。
func localizeCommandBasics(command *cobra.Command) {
	command.SetHelpTemplate(localizedHelpTemplate)
	command.SetUsageTemplate(localizedUsageTemplate)
	command.InitDefaultHelpFlag()
	if helpFlag := command.Flags().Lookup("help"); helpFlag != nil {
		helpFlag.Usage = "show help information"
	}
}

// localizeCommandHelp 统一 CLI help 模板，降低中文用户阅读成本。
func localizeCommandHelp(command *cobra.Command) {
	localizeCommandBasics(command)
	var helpCommand *cobra.Command
	if command.HasSubCommands() {
		helpCommand = newLocalizedHelpCommand()
		localizeCommandBasics(helpCommand)
		command.SetHelpCommand(helpCommand)
	}
	for _, child := range command.Commands() {
		if child == helpCommand {
			continue
		}
		localizeCommandHelp(child)
	}
}

// newLocalizedHelpCommand 创建中文 help 子命令。
func newLocalizedHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "help [command]",
		Short:   "Show command help",
		Long:    "Show help for any command.",
		GroupID: commandGroupHelp,
		RunE: func(cmd *cobra.Command, args []string) error {
			parent := cmd.Parent()
			if parent == nil {
				parent = cmd.Root()
			}
			target := parent
			if len(args) > 0 {
				found, _, err := parent.Find(args)
				if err != nil || found == nil {
					return fmt.Errorf("unknown help topic %q", strings.Join(args, " "))
				}
				target = found
			}
			return target.Help()
		},
	}
}

// setCommandGroup 将指定子命令绑定到 usage 功能分组。
func setCommandGroup(parent *cobra.Command, groupID string, names ...string) {
	for _, name := range names {
		mustCommand(parent, name).GroupID = groupID
	}
}

// newLogger 创建 Zerolog 基础 logger，供后续任务从命令上下文中复用。
func newLogger(output io.Writer, component string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339
	return zerolog.New(output).With().Timestamp().Str("component", component).Logger()
}

// validateServiceManager 校验服务管理器全局参数是否在允许集合内。
func validateServiceManager(manager string) error {
	switch manager {
	case defaultServiceManager, serviceManagerSystemd, serviceManagerLaunchd:
		return nil
	default:
		return fmt.Errorf("unsupported service-manager %q; supported values: auto, systemd, launchd", manager)
	}
}

// validateListenAddress 校验 sboxsub 全局监听地址，空值表示使用配置默认值。
func validateListenAddress(listen string) error {
	if listen == "" {
		return nil
	}

	host, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("listen must use HOST:PORT format: %w", err)
	}
	if host == "" {
		return fmt.Errorf("listen host cannot be empty")
	}

	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("listen port must be an integer from 1 to 65535: %w", err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("listen port must be in range 1-65535")
	}
	return nil
}

// newVersionCommand 创建版本命令，组件级版本参数保留给后续任务实现。
func newVersionCommand(use string, allowComponent bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: "Print the current binary version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return runComponentVersion(cmd, args[0])
			}

			info := version.Get()
			return writeSectionFields(cmd, "Version",
				outputKV("Version", info.Version),
				outputKV("Commit", info.Commit),
				outputKV("Build time", info.BuildTime),
			)
		},
	}
	if allowComponent {
		cmd.ValidArgs = []string{"sing-box", "rules"}
		cmd.Args = cobra.MaximumNArgs(1)
		return cmd
	}

	cmd.Args = cobra.NoArgs
	return cmd
}

// newStubCommand 创建后续任务的占位命令，执行时返回统一未实现错误。
func newStubCommand(use string, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplementedError(cmd)
		},
	}
}

// newStubGroup 创建带子命令的占位分组，直接执行分组时同样返回未实现错误。
func newStubGroup(use string, short string, children ...*cobra.Command) *cobra.Command {
	cmd := newStubCommand(use, short)
	cmd.AddCommand(children...)
	return cmd
}

// notImplementedError 返回携带具体命令路径的未实现错误。
func notImplementedError(cmd *cobra.Command) error {
	return fmt.Errorf("%w: %s is not available yet", ErrNotImplemented, cmd.CommandPath())
}
