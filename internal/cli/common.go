package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
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
		if _, writeErr := fmt.Fprintf(cmd.ErrOrStderr(), "错误：%v\n", err); writeErr != nil {
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

	root.PersistentFlags().StringVar(&opts.baseDir, "base-dir", defaultBaseDir, "环境目录")
	root.PersistentFlags().StringVar(&opts.serviceManager, "service-manager", defaultServiceManager, "服务管理器：auto、systemd、launchd")
	if includeListen {
		root.PersistentFlags().StringVar(&opts.listen, "listen", "", "覆盖订阅 HTTP 监听地址，格式 HOST:PORT")
	}

	return root
}

// getRootOptions 从命令上下文读取全局参数。
func getRootOptions(cmd *cobra.Command) (*rootOptions, error) {
	options, ok := cmd.Context().Value(rootOptionsContextKey{}).(*rootOptions)
	if !ok || options == nil {
		return nil, fmt.Errorf("读取 CLI 全局参数失败")
	}
	return options, nil
}

// addCommandGroups 为父命令注册 usage 中展示的功能分组。
func addCommandGroups(parent *cobra.Command, groups ...*cobra.Group) {
	parent.AddGroup(groups...)
	parent.SetHelpCommandGroupID(commandGroupHelp)
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
		return fmt.Errorf("不支持的 service-manager %q，仅支持 auto、systemd、launchd", manager)
	}
}

// validateListenAddress 校验 sboxsub 全局监听地址，空值表示使用配置默认值。
func validateListenAddress(listen string) error {
	if listen == "" {
		return nil
	}

	host, portText, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("listen 必须是 HOST:PORT 格式: %w", err)
	}
	if host == "" {
		return fmt.Errorf("listen host 不能为空")
	}

	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("listen port 必须是 1-65535 的整数: %w", err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("listen port 必须在 1-65535 范围内")
	}
	return nil
}

// newVersionCommand 创建版本命令，组件级版本参数保留给后续任务实现。
func newVersionCommand(use string, allowComponent bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: "输出当前二进制版本信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return runComponentVersion(cmd, args[0])
			}

			info := version.Get()
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Version: %s\nCommit: %s\nBuildTime: %s\n", info.Version, info.Commit, info.BuildTime); err != nil {
				return fmt.Errorf("write version output: %w", err)
			}
			return nil
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
	return fmt.Errorf("%w: %s", ErrNotImplemented, cmd.CommandPath())
}
