package cli

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/diagnostics"
	"github.com/sunliang711/sbox-manager/internal/domain"
	instancemgr "github.com/sunliang711/sbox-manager/internal/instance"
	"github.com/sunliang711/sbox-manager/internal/service"
	"github.com/sunliang711/sbox-manager/internal/subscription"
	"github.com/sunliang711/sbox-manager/internal/subserver"
)

var newSboxsubServiceManager = func(options *rootOptions) (*service.Manager, error) {
	return service.NewManager(service.Options{Kind: options.serviceManager})
}

// newSboxsubInitCommandT05 创建订阅服务初始化命令。
func newSboxsubInitCommandT05() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "初始化订阅服务环境目录和默认配置",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			force, _ := cmd.Flags().GetBool("force")
			if err := initSubBaseDir(options.baseDir, force); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "初始化完成: %s\n", options.baseDir)
			return err
		},
	}
}

// newSboxsubConfigCommandT05 创建订阅服务配置命令组。
func newSboxsubConfigCommandT05() *cobra.Command {
	configCommand := &cobra.Command{
		Use:   "config",
		Short: "编辑、展示或检查订阅服务配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			editor, _ := cmd.Flags().GetString("editor")
			return editSubConfigCommand(cmd, editor)
		},
	}
	configCommand.AddCommand(
		newSboxsubConfigShowCommand(),
		newSboxsubConfigCheckCommand(),
	)
	return configCommand
}

// newSboxsubImportCommandT05 创建订阅 bundle 导入命令。
func newSboxsubImportCommandT05() *cobra.Command {
	return &cobra.Command{
		Use:   "import BUNDLE",
		Short: "导入订阅 bundle",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			replaceAll, _ := cmd.Flags().GetBool("replace-all")
			result, err := subscription.ImportBundle(options.baseDir, args[0], replaceAll)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 bundle 已导入: inputs=%d nodes=%d replace_all=%t\n", result.Inputs, result.Nodes, result.Replace)
			return err
		},
	}
}

// newSboxsubClearCommandT05 创建清空订阅 input 命令。
func newSboxsubClearCommandT05() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "清空订阅服务数据",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			inputDir := subscription.InputsDir(options.baseDir)
			if err := os.RemoveAll(inputDir); err != nil {
				return fmt.Errorf("清空 input 目录: %w", err)
			}
			if err := os.MkdirAll(inputDir, 0750); err != nil {
				return fmt.Errorf("创建 input 目录: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "订阅 input 已清空")
			return err
		},
	}
}

// newSboxsubInputCommandT05 创建订阅 input 管理命令组。
func newSboxsubInputCommandT05() *cobra.Command {
	input := &cobra.Command{
		Use:   "input",
		Short: "管理订阅服务输入源",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	input.AddCommand(
		newSboxsubInputListCommand(),
		newSboxsubInputShowCommand(),
		newSboxsubInputValidateCommand(),
		newSboxsubInputEditCommand(),
		newSboxsubInputCloneCommand(),
		newSboxsubInputSetHostCommand(),
		newSboxsubInputRemoveCommand(),
	)
	mustCommand(input, "show").Flags().Bool("raw", false, "输出原始内容")
	mustCommand(input, "show").Flags().Bool("show-secrets", false, "显示敏感字段明文")
	mustCommand(input, "edit").Flags().String("editor", "", "指定编辑器命令")
	mustCommand(input, "clone").Flags().String("editor", "", "指定编辑器命令")
	mustCommand(input, "set-host").Flags().Bool("all", false, "应用到全部输入源")
	return input
}

// newSboxsubServeCommandT05 创建订阅 HTTP 服务命令。
func newSboxsubServeCommandT05() *cobra.Command {
	serve := &cobra.Command{
		Use:   "serve",
		Short: "启动订阅 HTTP 服务",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			subConfig, err := loadSubConfigFromCommand(cmd)
			if err != nil {
				return err
			}
			host, _ := cmd.Flags().GetString("host")
			port, _ := cmd.Flags().GetInt("port")
			if err := applyServeListenOverride(subConfig, host, port); err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return subserver.Run(ctx, subserver.Options{
				BaseDir: options.baseDir,
				Config:  *subConfig,
				Logger:  options.logger,
			})
		},
	}
	serve.Flags().String("host", "", "覆盖 HTTP 监听 host")
	serve.Flags().Int("port", 0, "覆盖 HTTP 监听 port")
	return serve
}

// newSboxsubServiceCommandT05 创建订阅服务文件命令组。
func newSboxsubServiceCommandT05() *cobra.Command {
	serviceCommand := &cobra.Command{
		Use:   "service",
		Short: "管理订阅服务 systemd unit 或 launchd plist",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	serviceCommand.AddCommand(
		newSboxsubServiceInstallCommand(),
		newSboxsubServiceUninstallCommand(),
	)
	return serviceCommand
}

// newSboxsubServiceInstallCommand 创建订阅服务安装命令。
func newSboxsubServiceInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "安装订阅服务文件",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			manager, err := newSboxsubServiceManager(options)
			if err != nil {
				return err
			}
			binary, err := os.Executable()
			if err != nil {
				return fmt.Errorf("解析当前 sboxsub binary: %w", err)
			}
			if err := os.MkdirAll(filepath.Join(options.baseDir, "logs"), 0750); err != nil {
				return fmt.Errorf("创建日志目录: %w", err)
			}
			if err := manager.InstallSubscription(cmd.Context(), options.baseDir, binary); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "订阅服务文件已安装")
			return err
		},
	}
}

// newSboxsubServiceUninstallCommand 创建订阅服务卸载命令。
func newSboxsubServiceUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "卸载订阅服务文件",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			manager, err := newSboxsubServiceManager(options)
			if err != nil {
				return err
			}
			if err := manager.UninstallSubscription(cmd.Context()); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "订阅服务文件已卸载")
			return err
		},
	}
}

// newSboxsubServiceActionCommand 创建订阅服务生命周期命令。
func newSboxsubServiceActionCommand(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action,
		Short: sboxsubServiceActionShort(action),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			manager, err := newSboxsubServiceManager(options)
			if err != nil {
				return err
			}
			follow, _ := cmd.Flags().GetBool("follow")
			serviceName := service.SubscriptionServiceNameForKind(manager.Kind())
			results, err := manager.Run(cmd.Context(), action, []string{serviceName}, follow)
			if err != nil {
				return err
			}
			if err := writeServiceActionOutput(cmd, results); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s 完成: %s\n", action, serviceName)
			return err
		},
	}
}

// newSboxsubDoctorCommandT05 创建订阅服务诊断命令。
func newSboxsubDoctorCommandT05() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "执行订阅服务诊断检查",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			checks := diagnostics.SubDoctor(cmd.Context(), options.baseDir, options.serviceManager, options.listen)
			if err := writeDiagnosticChecks(cmd, checks); err != nil {
				return err
			}
			if diagnostics.HasIssue(checks) {
				return fmt.Errorf("doctor found ISSUE")
			}
			return nil
		},
	}
}

// newSboxsubConfigShowCommand 创建 config show 子命令。
func newSboxsubConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "展示订阅服务配置",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			subConfig, err := loadSubConfigFromCommand(cmd)
			if err != nil {
				return err
			}
			showSecrets, _ := cmd.Flags().GetBool("show-secrets")
			data, err := subscription.MarshalStable(newSubConfigView(*subConfig, showSecrets))
			if err != nil {
				return err
			}
			return writeOutput(cmd, data)
		},
	}
}

// newSboxsubConfigCheckCommand 创建 config check 子命令。
func newSboxsubConfigCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "检查订阅服务配置",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadSubConfigFromCommand(cmd); err != nil {
				return err
			}
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "订阅服务配置校验通过")
			return err
		},
	}
}

// newSboxsubInputListCommand 创建 input list 子命令。
func newSboxsubInputListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出输入源",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			files, err := subscription.LoadInputsFromDir(subscription.InputsDir(options.baseDir))
			if err != nil {
				return err
			}
			for _, file := range files {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), file.Name); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// newSboxsubInputShowCommand 创建 input show 子命令。
func newSboxsubInputShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show SOURCE",
		Short: "展示输入源",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			path, err := inputPath(options.baseDir, args[0])
			if err != nil {
				return err
			}
			raw, _ := cmd.Flags().GetBool("raw")
			showSecrets, _ := cmd.Flags().GetBool("show-secrets")
			if raw && showSecrets {
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return writeOutput(cmd, data)
			}
			input, err := config.LoadSubscriptionInput(path)
			if err != nil {
				return err
			}
			value := *input
			if !showSecrets {
				value = subscription.RedactInput(value)
			}
			data, err := subscription.MarshalStable(value)
			if err != nil {
				return err
			}
			return writeOutput(cmd, data)
		},
	}
}

// newSboxsubInputValidateCommand 创建 input validate 子命令。
func newSboxsubInputValidateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate [SOURCE]",
		Short: "校验输入源",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				path, err := inputPath(options.baseDir, args[0])
				if err != nil {
					return err
				}
				if _, err := config.LoadSubscriptionInput(path); err != nil {
					return err
				}
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 input 校验通过: %s\n", args[0])
				return err
			}
			index, err := subscription.LoadIndexFromDir(subscription.InputsDir(options.baseDir))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 input 校验通过: sources=%d users=%d nodes=%d\n", len(index.Sources), index.UserCount(), len(index.Nodes))
			return err
		},
	}
}

// newSboxsubInputEditCommand 保留编辑入口到后续任务。
func newSboxsubInputEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit SOURCE",
		Short: "编辑输入源",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			editor, _ := cmd.Flags().GetString("editor")
			if err := editSubInput(options.baseDir, args[0], editor); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 input 已更新: %s\n", args[0])
			return err
		},
	}
}

// newSboxsubInputCloneCommand 创建 input clone 子命令。
func newSboxsubInputCloneCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clone SOURCE TARGET",
		Short: "克隆输入源",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			sourcePath, err := inputPath(options.baseDir, args[0])
			if err != nil {
				return err
			}
			targetPath, err := inputPath(options.baseDir, args[1])
			if err != nil {
				return err
			}
			if _, err := os.Stat(targetPath); err == nil {
				return fmt.Errorf("target 已存在: %s", args[1])
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			data, err := os.ReadFile(sourcePath)
			if err != nil {
				return err
			}
			cloneData, err := retargetClonedInput(args[0], args[1], data)
			if err != nil {
				return err
			}
			editor, _ := cmd.Flags().GetString("editor")
			if err := writeEditableInputClone(options.baseDir, args[1], cloneData, editor); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 input 已克隆: %s -> %s\n", args[0], args[1])
			return err
		},
	}
}

// newSboxsubInputSetHostCommand 创建 input set-host 子命令。
func newSboxsubInputSetHostCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-host HOST [SOURCE]",
		Short: "设置输入源 external host",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			all, _ := cmd.Flags().GetBool("all")
			if !all && len(args) != 2 {
				return fmt.Errorf("未指定 --all 时必须提供 SOURCE")
			}
			names := []string{}
			if all {
				files, err := subscription.LoadInputsFromDir(subscription.InputsDir(options.baseDir))
				if err != nil {
					return err
				}
				names = names[:0]
				for _, file := range files {
					names = append(names, file.Name)
				}
			} else {
				names = []string{args[1]}
			}
			for _, name := range names {
				if err := updateInputHost(options.baseDir, name, args[0]); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 input external_host 已更新: %d\n", len(names))
			return err
		},
	}
}

// newSboxsubInputRemoveCommand 创建 input remove 子命令。
func newSboxsubInputRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove SOURCE",
		Short: "移除输入源",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			path, err := inputPath(options.baseDir, args[0])
			if err != nil {
				return err
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 input 已移除: %s\n", args[0])
			return err
		},
	}
}

// initSubBaseDir 创建订阅服务目录和默认配置。
func initSubBaseDir(baseDir string, force bool) error {
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("base dir 不能为空")
	}
	for _, dir := range []string{
		baseDir,
		subscription.InputsDir(baseDir),
		filepath.Join(baseDir, "templates", "sub"),
		filepath.Join(baseDir, "logs"),
	} {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("创建目录 %s: %w", dir, err)
		}
	}
	configPath := filepath.Join(baseDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil && !force {
		return fmt.Errorf("配置文件 %s 已存在，覆盖请使用 --force", configPath)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return subscription.WriteFileAtomic(configPath, []byte(defaultSubConfigYAML), 0640)
}

// loadSubConfigFromCommand 根据 CLI 全局参数加载订阅服务配置。
func loadSubConfigFromCommand(cmd *cobra.Command) (*domain.SubConfig, error) {
	options, err := getRootOptions(cmd)
	if err != nil {
		return nil, err
	}
	subConfig, err := config.LoadSubConfig(filepath.Join(options.baseDir, "config.yaml"), options.baseDir)
	if err != nil {
		return nil, err
	}
	if options.listen != "" {
		subConfig.Listen = options.listen
	}
	if err := domain.ValidateSubConfig(*subConfig); err != nil {
		return nil, err
	}
	return subConfig, nil
}

// applyServeListenOverride 应用 serve --host/--port 覆盖。
func applyServeListenOverride(subConfig *domain.SubConfig, host string, port int) error {
	if host == "" && port == 0 {
		return nil
	}
	currentHost, currentPort, err := net.SplitHostPort(subConfig.Listen)
	if err != nil {
		return err
	}
	if host != "" {
		currentHost = host
	}
	if port != 0 {
		if port < 1 || port > 65535 {
			return fmt.Errorf("port 必须在 1-65535 范围内")
		}
		currentPort = strconv.Itoa(port)
	}
	subConfig.Listen = net.JoinHostPort(currentHost, currentPort)
	return domain.ValidateSubConfig(*subConfig)
}

// inputPath 返回安全订阅 input 文件路径。
func inputPath(baseDir string, name string) (string, error) {
	if err := domain.ValidateSubscriptionInputFilename(name); err != nil {
		return "", err
	}
	return filepath.Join(subscription.InputsDir(baseDir), name), nil
}

// updateInputHost 更新单个 input 的 external_host 并写回。
func updateInputHost(baseDir string, name string, host string) error {
	path, err := inputPath(baseDir, name)
	if err != nil {
		return err
	}
	input, err := config.LoadSubscriptionInput(path)
	if err != nil {
		return err
	}
	input.ExternalHost = host
	if err := domain.ValidateSubscriptionInput(*input); err != nil {
		return err
	}
	data, err := subscription.MarshalStable(input)
	if err != nil {
		return err
	}
	return subscription.WriteFileAtomic(path, data, 0640)
}

// retargetClonedInput 将克隆 input 中依赖 source 的唯一字段改写为目标名称。
func retargetClonedInput(sourceName string, targetName string, data []byte) ([]byte, error) {
	input, err := subscription.DecodeInput(sourceName, data)
	if err != nil {
		return nil, err
	}
	source := input.Source
	target := inputSourceFromFilename(targetName)
	input.Source = target
	for index := range input.Nodes {
		input.Nodes[index].ID = retargetCloneValue(input.Nodes[index].ID, source+":", target+":")
		input.Nodes[index].Tag = retargetCloneValue(input.Nodes[index].Tag, source+"-", target+"-")
		input.Nodes[index].Remark = retargetCloneRemark(input.Nodes[index].Remark, source, target)
	}
	return subscription.MarshalStable(input)
}

// inputSourceFromFilename 从安全 input 文件名推导 source 值。
func inputSourceFromFilename(name string) string {
	extension := filepath.Ext(name)
	return strings.TrimSuffix(filepath.Base(name), extension)
}

// retargetCloneValue 替换旧前缀；没有旧前缀时为避免冲突添加目标前缀。
func retargetCloneValue(value string, oldPrefix string, newPrefix string) string {
	if strings.HasPrefix(value, oldPrefix) {
		return newPrefix + strings.TrimPrefix(value, oldPrefix)
	}
	return newPrefix + value
}

// retargetCloneRemark 为克隆 input 生成同 user 下不重复的展示名。
func retargetCloneRemark(value string, source string, target string) string {
	if value == "" || value == source {
		return target
	}
	return target + " " + value
}

// editSubConfigCommand 编辑 sboxsub config 并在替换前严格校验。
func editSubConfigCommand(cmd *cobra.Command, editor string) error {
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	path := filepath.Join(options.baseDir, "config.yaml")
	draft := draftPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	if err := os.WriteFile(draft, data, 0600); err != nil {
		return fmt.Errorf("写入草稿文件 %s: %w", draft, err)
	}
	defer os.Remove(draft)
	if err := instancemgr.EditFileWithCommand(draft, editor); err != nil {
		return err
	}
	if _, err := config.LoadSubConfig(draft, options.baseDir); err != nil {
		return err
	}
	if err := os.Rename(draft, path); err != nil {
		return fmt.Errorf("替换配置文件 %s: %w", path, err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅服务配置已更新: %s\n", path)
	return err
}

// editSubInput 编辑单个 input 并在替换前校验整体 index。
func editSubInput(baseDir string, name string, editor string) error {
	path, err := inputPath(baseDir, name)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取订阅 input %s: %w", path, err)
	}
	return editInputData(baseDir, name, data, editor)
}

// writeEditableInputClone 编辑 clone 草稿，校验通过后写入目标 input。
func writeEditableInputClone(baseDir string, target string, data []byte, editor string) error {
	return editInputData(baseDir, target, data, editor)
}

// editInputData 将数据写成草稿，调用 editor 后校验并原子写入。
func editInputData(baseDir string, name string, data []byte, editor string) error {
	targetPath, err := inputPath(baseDir, name)
	if err != nil {
		return err
	}
	draft := draftPath(targetPath)
	if err := os.WriteFile(draft, data, 0600); err != nil {
		return fmt.Errorf("写入草稿文件 %s: %w", draft, err)
	}
	defer os.Remove(draft)
	if err := instancemgr.EditFileWithCommand(draft, editor); err != nil {
		return err
	}
	draftData, err := os.ReadFile(draft)
	if err != nil {
		return fmt.Errorf("读取草稿文件 %s: %w", draft, err)
	}
	input, err := subscription.DecodeInput(name, draftData)
	if err != nil {
		return err
	}
	if err := validateInputsWithReplacement(baseDir, name, input); err != nil {
		return err
	}
	return subscription.WriteFileAtomic(targetPath, draftData, 0640)
}

// validateInputsWithReplacement 校验替换目标 input 后的整体 index 约束。
func validateInputsWithReplacement(baseDir string, name string, input domain.SubscriptionInput) error {
	files, err := subscription.LoadInputsFromDir(subscription.InputsDir(baseDir))
	if err != nil {
		return err
	}
	inputs := make([]domain.SubscriptionInput, 0, len(files)+1)
	replaced := false
	for _, file := range files {
		if file.Name == name {
			inputs = append(inputs, input)
			replaced = true
			continue
		}
		inputs = append(inputs, file.Input)
	}
	if !replaced {
		inputs = append(inputs, input)
	}
	_, err = subscription.BuildIndexFromInputs(inputs)
	return err
}

// writeServiceActionOutput 输出服务管理器返回内容。
func writeServiceActionOutput(cmd *cobra.Command, results []service.Result) error {
	for _, result := range results {
		if len(result.Output) == 0 {
			continue
		}
		if _, err := cmd.OutOrStdout().Write(result.Output); err != nil {
			return err
		}
		if !strings.HasSuffix(string(result.Output), "\n") {
			if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
				return err
			}
		}
	}
	return nil
}

// sboxsubServiceActionShort 返回订阅服务生命周期命令说明。
func sboxsubServiceActionShort(action string) string {
	switch action {
	case "start":
		return "启动订阅服务"
	case "stop":
		return "停止订阅服务"
	case "restart":
		return "重启订阅服务"
	case "status":
		return "查看订阅服务状态"
	case "logs":
		return "查看订阅服务日志"
	case "enable":
		return "启用订阅服务"
	case "disable":
		return "禁用订阅服务"
	default:
		return "管理订阅服务"
	}
}

// newSubConfigView 生成默认脱敏的配置展示结构。
func newSubConfigView(subConfig domain.SubConfig, showSecrets bool) subConfigView {
	token := subConfig.Access.Token
	if !showSecrets && token != "" {
		token = "[REDACTED]"
	}
	return subConfigView{
		Version:       subConfig.Version,
		Listen:        subConfig.Listen,
		Access:        domain.AccessConfig{Type: subConfig.Access.Type, Token: token},
		TemplatesDir:  subConfig.TemplatesDir,
		WatchInterval: subConfig.WatchInterval.String(),
		WatchDebounce: subConfig.WatchDebounce.String(),
		ManagedConfig: subConfig.ManagedConfig,
	}
}

type subConfigView struct {
	Version       int                  `json:"version"`
	Listen        string               `json:"listen"`
	Access        domain.AccessConfig  `json:"access"`
	TemplatesDir  string               `json:"templates_dir"`
	WatchInterval string               `json:"watch_interval"`
	WatchDebounce string               `json:"watch_debounce"`
	ManagedConfig domain.ManagedConfig `json:"managed_config"`
}

const defaultSubConfigYAML = `# sboxsub subscription service config.
# version: 配置版本，目前固定为 1。
version: 1

# listen: HTTP 监听地址。默认只监听本机；公网监听时建议设置 access.type=token。
listen: 127.0.0.1:3003

# access: 订阅访问控制。
#   type: none 表示不鉴权；token 表示必须通过 /FORMAT/TOKEN/USER 或 ?token= 提供 token。
access:
  type: none
  # token: change-me

# templates_dir: 自定义订阅模板目录；相对路径按 sboxsub base-dir 解析。
templates_dir: templates

# watch_interval/watch_debounce: input 文件轮询间隔和变更防抖时间。
watch_interval: 2s
watch_debounce: 300ms

# managed_config: Surge Managed Config 输出参数。
managed_config:
  enabled: true
  # public_base_url: https://sub.example.com
  interval: 86400
  strict: true
`
