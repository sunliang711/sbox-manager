package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
	installer "github.com/sunliang711/sbox-manager/internal/install"
	instancemgr "github.com/sunliang711/sbox-manager/internal/instance"
	"github.com/sunliang711/sbox-manager/internal/service"
)

type resourceInstallerRunner interface {
	Run(ctx context.Context, global domain.GlobalConfig, options installer.Options) error
}

var newResourceInstaller = func() resourceInstallerRunner {
	return installer.NewInstaller()
}

// newSboxctlInitCommand 创建 init 命令。
func newSboxctlInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "初始化 agent 环境目录和默认配置",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			externalHost, _ := cmd.Flags().GetString("external-host")
			force, _ := cmd.Flags().GetBool("force")
			if err := instancemgr.Init(options.baseDir, instancemgr.InitOptions{ExternalHost: externalHost, Force: force}); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "初始化完成: %s\n%s", options.baseDir, sboxctlInitNextSteps(options.baseDir))
			return err
		},
	}
}

// newSboxctlSetupCommand 创建 setup 命令。
func newSboxctlSetupCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "初始化并安装 agent 运行依赖",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			externalHost, _ := cmd.Flags().GetString("external-host")
			force, _ := cmd.Flags().GetBool("force")
			start, _ := cmd.Flags().GetBool("start")
			if err := instancemgr.Init(options.baseDir, instancemgr.InitOptions{ExternalHost: externalHost, Force: force, AllowExisting: true}); err != nil {
				return err
			}
			set, err := config.LoadAgentConfigSet(options.baseDir)
			if err != nil {
				return err
			}
			if err := newResourceInstaller().Run(cmd.Context(), set.Global, installer.Options{
				Operation: installer.OperationInstall,
				Resource:  installer.ResourceAll,
				Progress:  installProgress(cmd),
			}); err != nil {
				return err
			}
			manager, err := newSboxctlServiceManager(options)
			if err != nil {
				return err
			}
			if err := installSboxctlServiceFiles(cmd, manager, set, ""); err != nil {
				return err
			}
			if start {
				return runSboxctlRuntimeLifecycle(cmd, "start", nil)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "setup 完成\n%s", sboxctlSetupNextSteps(options.baseDir))
			return err
		},
	}
}

// sboxctlInitNextSteps 返回 agent 初始化后的下一步提示。
func sboxctlInitNextSteps(baseDir string) string {
	return fmt.Sprintf("下一步：\n- 为了创建实例配置，执行：sboxctl --base-dir %s add edge-us --template edge\n- 为了安装 sing-box、规则集和服务文件，执行：sudo sboxctl --base-dir %s setup\n- 不确定还缺什么，执行：sboxctl --base-dir %s doctor\n", baseDir, baseDir, baseDir)
}

// sboxctlSetupNextSteps 返回 agent setup 后的下一步提示。
func sboxctlSetupNextSteps(baseDir string) string {
	return fmt.Sprintf("下一步：\n- 为了启动已启用实例，执行：sudo sboxctl --base-dir %s start\n- 为了自动采集 traffic，执行：sudo sboxctl --base-dir %s traffic timer enable\n- 不确定还缺什么，执行：sboxctl --base-dir %s doctor\n", baseDir, baseDir, baseDir)
}

// newSboxctlConfigCommand 创建 agent 配置命令。
func newSboxctlConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config [INSTANCE]",
		Short: "编辑或检查 agent 配置",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check-only")
			editor, _ := cmd.Flags().GetString("editor")
			if checkOnly {
				if err := checkConfigCommand(cmd, args); err != nil {
					return err
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "配置校验通过")
				return err
			}
			return editConfigCommand(cmd, args, editor)
		},
	}
}

// newSboxctlExampleCommand 创建 example 命令。
func newSboxctlExampleCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "example [global|instance|inbound|outbound|group|route|traffic] [TYPE]",
		Short: "输出配置示例片段",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := "global"
			if len(args) > 0 {
				kind = args[0]
			}
			typeName := ""
			if len(args) > 1 {
				typeName = args[1]
			}
			text, err := exampleSnippet(kind, typeName)
			if err != nil {
				return err
			}
			if !strings.HasSuffix(text, "\n") {
				text += "\n"
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), text)
			return err
		},
	}
}

// newSboxctlAddCommand 创建 add 命令。
func newSboxctlAddCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "add NAME",
		Short: "新增 sing-box 实例配置",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			template, _ := cmd.Flags().GetString("template")
			fromFile, _ := cmd.Flags().GetString("from-file")
			allocate, _ := cmd.Flags().GetBool("allocate-ports")
			keep, _ := cmd.Flags().GetBool("keep-template-ports")
			edit, _ := cmd.Flags().GetBool("edit")
			noEdit, _ := cmd.Flags().GetBool("no-edit")
			editor, _ := cmd.Flags().GetString("editor")
			created, err := instancemgr.Add(options.baseDir, instancemgr.AddOptions{
				Name:              args[0],
				Template:          template,
				FromFile:          fromFile,
				AllocatePorts:     allocate,
				KeepTemplatePorts: keep,
			})
			if err != nil {
				return err
			}
			if edit && !noEdit {
				if err := editInstanceByName(options.baseDir, created.Name, editor); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "instance 已创建: %s\n", created.Name)
			return err
		},
	}
}

// newSboxctlListCommand 创建 list 命令。
func newSboxctlListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "列出 agent 管理的实例",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			for _, line := range instancemgr.ListLines(set.Instances, verbose) {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// newSboxctlCloneCommand 创建 clone 命令。
func newSboxctlCloneCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clone SOURCE TARGET",
		Short: "克隆实例配置",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			allocate, _ := cmd.Flags().GetBool("allocate-ports")
			edit, _ := cmd.Flags().GetBool("edit")
			noEdit, _ := cmd.Flags().GetBool("no-edit")
			editor, _ := cmd.Flags().GetString("editor")
			cloned, err := instancemgr.Clone(options.baseDir, instancemgr.CloneOptions{Source: args[0], Target: args[1], AllocatePorts: allocate})
			if err != nil {
				return err
			}
			if edit && !noEdit {
				if err := editInstanceByName(options.baseDir, cloned.Name, editor); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "instance 已克隆: %s\n", cloned.Name)
			return err
		},
	}
}

// newSboxctlMemberCommand 创建 group member 管理命令。
func newSboxctlMemberCommand() *cobra.Command {
	member := &cobra.Command{
		Use:   "member",
		Short: "维护 selector 或 urltest group 成员",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	member.AddCommand(
		&cobra.Command{
			Use:   "list INSTANCE GROUP",
			Short: "列出 group 成员",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				set, err := loadAgentSetFromCommand(cmd)
				if err != nil {
					return err
				}
				members, err := instancemgr.MemberList(set, args[0], args[1])
				if err != nil {
					return err
				}
				for _, member := range members {
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), member); err != nil {
						return err
					}
				}
				return nil
			},
		},
		memberMutationCommand("add", "添加 group 成员", instancemgr.MemberAdd),
		memberMutationCommand("remove", "移除 group 成员", instancemgr.MemberRemove),
	)
	return member
}

func memberMutationCommand(name string, short string, mutate func(string, string, string, string) error) *cobra.Command {
	return &cobra.Command{
		Use:   name + " INSTANCE GROUP MEMBER",
		Short: short,
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			if err := mutate(options.baseDir, args[0], args[1], args[2]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "member %s 完成\n", name)
			return err
		},
	}
}

// newSboxctlRemoveCommand 创建 remove 命令。
func newSboxctlRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "移除实例配置",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			purge, _ := cmd.Flags().GetBool("purge")
			if err := instancemgr.Remove(options.baseDir, args[0], purge); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "instance 已移除: %s\n", args[0])
			return err
		},
	}
}

// newSboxctlServiceActionCommand 创建只调用服务管理器的顶层生命周期命令。
func newSboxctlServiceActionCommand(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " [TARGET]",
		Short: serviceActionShort(action),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServiceActionCommand(cmd, action, optionalArg(args))
		},
	}
}

// newSboxctlServiceCommand 创建服务文件和服务生命周期命令组。
func newSboxctlServiceCommand() *cobra.Command {
	serviceCommand := &cobra.Command{
		Use:   "service",
		Short: "管理实例 systemd 模板 unit 或 launchd plist",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	serviceCommand.AddCommand(
		newSboxctlServiceInstallCommand(),
		newSboxctlServiceUninstallCommand(),
		serviceSubActionCommand("start", "启动实例服务"),
		serviceSubActionCommand("stop", "停止实例服务"),
		serviceSubActionCommand("restart", "重启实例服务"),
		serviceSubActionCommand("status", "查看实例服务状态"),
		serviceSubActionCommand("logs", "查看实例服务日志"),
		serviceSubActionCommand("log", "查看实例服务日志"),
		serviceSubActionCommand("enable", "启用实例服务"),
		serviceSubActionCommand("disable", "禁用实例服务"),
	)
	mustCommand(serviceCommand, "logs").Flags().BoolP("follow", "f", false, "持续跟随日志")
	mustCommand(serviceCommand, "log").Flags().BoolP("follow", "f", false, "持续跟随日志")
	return serviceCommand
}

func newSboxctlServiceInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install [TARGET]",
		Short: "安装实例服务文件",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, set, manager, err := loadServiceCommandContext(cmd)
			if err != nil {
				return err
			}
			if err := installSboxctlServiceFiles(cmd, manager, set, optionalArg(args)); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "service install 完成: %s\n", manager.Kind())
			return err
		},
	}
}

// installSboxctlServiceFiles 安装实例服务文件，并同步安装 traffic timer 服务文件。
func installSboxctlServiceFiles(cmd *cobra.Command, manager *service.Manager, set *config.AgentConfigSet, target string) error {
	if err := manager.Install(cmd.Context(), set.BaseDir, set.Global, set.Instances, target); err != nil {
		return err
	}
	binary, err := trafficExecutablePath()
	if err != nil {
		return fmt.Errorf("解析 sboxctl 路径: %w", err)
	}
	return manager.InstallTrafficTimers(cmd.Context(), set.BaseDir, set.Global.Paths.Traffic, set.Global.Paths.Logs, binary)
}

func newSboxctlServiceUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall [TARGET]",
		Short: "卸载实例服务文件",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, set, manager, err := loadServiceCommandContext(cmd)
			if err != nil {
				return err
			}
			if err := manager.Uninstall(cmd.Context(), set.Instances, optionalArg(args)); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "service uninstall 完成")
			return err
		},
	}
}

func serviceSubActionCommand(action string, short string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " [TARGET]",
		Short: short,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServiceActionCommand(cmd, action, optionalArg(args))
		},
	}
}

func runServiceActionCommand(cmd *cobra.Command, action string, target string) error {
	_, set, manager, err := loadServiceCommandContext(cmd)
	if err != nil {
		return err
	}
	instances, err := service.SelectInstances(set.Instances, target)
	if err != nil {
		return err
	}
	services := manager.ServicesForInstances(instances)
	follow, _ := cmd.Flags().GetBool("follow")
	results, err := manager.Run(cmd.Context(), action, services, follow)
	if err != nil {
		return err
	}
	return writeServiceResults(cmd, action, results)
}

func loadServiceCommandContext(cmd *cobra.Command) (*rootOptions, *config.AgentConfigSet, *service.Manager, error) {
	options, err := getRootOptions(cmd)
	if err != nil {
		return nil, nil, nil, err
	}
	set, err := config.LoadAgentConfigSet(options.baseDir)
	if err != nil {
		return nil, nil, nil, err
	}
	manager, err := newSboxctlServiceManager(options)
	if err != nil {
		return nil, nil, nil, err
	}
	return options, set, manager, nil
}

func writeServiceResults(cmd *cobra.Command, action string, results []service.Result) error {
	if len(results) == 0 {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s 完成，无目标服务\n", action)
		return err
	}
	for _, result := range results {
		if len(result.Output) > 0 {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "==> %s\n%s", result.Service, result.Output); err != nil {
				return err
			}
			if !strings.HasSuffix(string(result.Output), "\n") {
				if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s 完成: %s\n", action, result.Service); err != nil {
			return err
		}
	}
	return nil
}

// newResourceCommand 创建 install/update/uninstall 共享资源命令。
func newResourceCommand(use string, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:       use + " sing-box|rules|all",
		Short:     short,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"sing-box", "rules", "all"},
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			version, _ := cmd.Flags().GetString("version")
			source, _ := cmd.Flags().GetString("source")
			sha256Text, _ := cmd.Flags().GetString("sha256")
			member, _ := cmd.Flags().GetString("archive-member")
			purge, _ := cmd.Flags().GetBool("purge")
			purgeAll := use == installer.OperationUninstall && purge && args[0] == installer.ResourceAll
			set, err := loadResourceCommandConfigSet(options.baseDir, purgeAll)
			if err != nil {
				return err
			}
			var manager *service.Manager
			if use == installer.OperationUninstall && purge {
				manager, err = newSboxctlServiceManager(options)
				if err != nil {
					return err
				}
				if purgeAll {
					if err := manager.StopInstancesForUninstall(cmd.Context(), set.Instances); err != nil {
						return err
					}
				}
			}
			err = newResourceInstaller().Run(cmd.Context(), set.Global, installer.Options{
				Operation:     use,
				Resource:      args[0],
				Version:       version,
				Source:        source,
				SHA256:        sha256Text,
				ArchiveMember: member,
				Purge:         purge,
				Progress:      installProgress(cmd),
			})
			if err != nil {
				return err
			}
			if use == installer.OperationUninstall && purge {
				if purgeAll {
					if err := manager.UninstallTrafficTimers(cmd.Context()); err != nil {
						return err
					}
					if err := manager.UninstallInstances(cmd.Context(), set.Instances); err != nil {
						return err
					}
					if err := purgeAgentBaseDir(set.BaseDir); err != nil {
						return err
					}
				} else if err := manager.Uninstall(cmd.Context(), set.Instances, ""); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s 完成\n", use, args[0])
			return err
		},
	}
	cmd.Flags().String("version", "", "目标版本")
	cmd.Flags().String("source", "", "下载源")
	cmd.Flags().String("sha256", "", "sha256 校验值")
	cmd.Flags().String("archive-member", "", "归档内成员名称")
	return cmd
}

// loadResourceCommandConfigSet 加载资源命令配置，允许全量 purge 重复执行时配置已不存在。
func loadResourceCommandConfigSet(baseDir string, allowPurgedBaseDir bool) (*config.AgentConfigSet, error) {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err == nil {
		return set, nil
	}
	if !allowPurgedBaseDir || !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return defaultAgentConfigSetForPurgedBaseDir(baseDir)
}

// defaultAgentConfigSetForPurgedBaseDir 返回 base-dir 已被删除后的默认空配置集合。
func defaultAgentConfigSetForPurgedBaseDir(baseDir string) (*config.AgentConfigSet, error) {
	resolvedBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("清理 base-dir %s: %w", baseDir, err)
	}
	resolvedBase = filepath.Clean(resolvedBase)
	global := domain.DefaultGlobalConfig()
	if err := config.NormalizeGlobalPaths(resolvedBase, &global); err != nil {
		return nil, err
	}
	return &config.AgentConfigSet{
		BaseDir:   resolvedBase,
		Global:    global,
		Instances: nil,
	}, nil
}

// purgeAgentBaseDir 删除 agent base-dir，并拒绝清理明显危险的根路径。
func purgeAgentBaseDir(baseDir string) error {
	cleaned := filepath.Clean(baseDir)
	if cleaned == "." || cleaned == string(os.PathSeparator) {
		return fmt.Errorf("拒绝清理危险 base-dir %s", cleaned)
	}
	exists, err := pathExists(cleaned)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err := os.RemoveAll(cleaned); err != nil {
		return fmt.Errorf("清理 agent base-dir %s: %w", cleaned, err)
	}
	return nil
}

// pathExists 判断 CLI 清理路径是否存在，路径不存在时不视为错误。
func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("检查路径 %s: %w", path, err)
	}
	return true, nil
}

func runSboxctlRuntimeLifecycle(cmd *cobra.Command, action string, args []string) error {
	lifecycle := newSboxctlLifecycleCommand(action)
	lifecycle.SetOut(cmd.OutOrStdout())
	lifecycle.SetErr(cmd.ErrOrStderr())
	lifecycle.SetArgs(args)
	lifecycle.SetContext(cmd.Context())
	return lifecycle.Execute()
}

func checkConfigCommand(cmd *cobra.Command, args []string) error {
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		_, err := config.LoadAgentConfigSet(options.baseDir)
		return err
	}
	set, err := config.LoadAgentConfigSet(options.baseDir)
	if err != nil {
		return err
	}
	path, err := instancemgr.FindInstancePath(set.Global, args[0])
	if err != nil {
		return err
	}
	_, err = config.LoadInstance(path, set.Global)
	return err
}

func editConfigCommand(cmd *cobra.Command, args []string, editor string) error {
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	set, err := config.LoadAgentConfigSet(options.baseDir)
	if err != nil {
		return err
	}
	path := filepath.Join(options.baseDir, "config.yaml")
	if len(args) == 1 {
		path, err = instancemgr.FindInstancePath(set.Global, args[0])
		if err != nil {
			return err
		}
	}
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
	if len(args) == 0 {
		loaded, err := config.LoadGlobalConfig(draft, options.baseDir)
		if err != nil {
			return err
		}
		instances, err := config.LoadInstances(loaded.Paths.Instances, *loaded)
		if err != nil {
			return err
		}
		if err := domain.ValidateConfigSet(*loaded, instances); err != nil {
			return err
		}
	} else if err := validateInstanceDraft(draft, filepath.Ext(path), set.Global); err != nil {
		return err
	}
	if err := os.Rename(draft, path); err != nil {
		return fmt.Errorf("替换配置文件 %s: %w", path, err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "配置已更新: %s\n", path)
	return err
}

func validateInstanceDraft(path string, extension string, global domain.GlobalConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取草稿文件 %s: %w", path, err)
	}
	instanceValue := domain.DefaultInstance(global)
	format := "yaml"
	if strings.EqualFold(extension, ".json") {
		format = "json"
	}
	if err := config.DecodeStrict(data, format, "Instance", &instanceValue); err != nil {
		return err
	}
	domain.ApplyInstanceDefaults(&instanceValue)
	return domain.ValidateInstance(global, &instanceValue)
}

func editInstanceByName(baseDir string, name string, editor string) error {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err != nil {
		return err
	}
	path, err := instancemgr.FindInstancePath(set.Global, name)
	if err != nil {
		return err
	}
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
	if err := validateInstanceDraft(draft, filepath.Ext(path), set.Global); err != nil {
		return err
	}
	if err := os.Rename(draft, path); err != nil {
		return fmt.Errorf("替换配置文件 %s: %w", path, err)
	}
	return nil
}

func draftPath(path string) string {
	extension := filepath.Ext(path)
	return strings.TrimSuffix(path, extension) + ".draft" + extension
}

func serviceActionShort(action string) string {
	switch action {
	case "stop":
		return "停止实例服务"
	case "status":
		return "查看实例服务状态"
	case "logs":
		return "查看实例服务日志"
	case "enable":
		return "启用实例服务"
	case "disable":
		return "禁用实例服务"
	default:
		return "管理实例服务"
	}
}

// installProgress 将资源安装器的英文进度日志输出到 stderr。
func installProgress(cmd *cobra.Command) installer.Progress {
	return func(message string) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), message)
	}
}

func exampleSnippet(kind string, typeName string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	typeName = strings.ToLower(strings.TrimSpace(typeName))
	switch kind {
	case "global":
		if typeName != "" {
			return "", unsupportedExampleType("global", typeName, []string{""})
		}
		return globalExampleSnippet(), nil
	case "instance":
		return instanceExampleSnippet(typeName)
	case "inbound":
		return inboundExampleSnippet(typeName)
	case "outbound":
		return outboundExampleSnippet(typeName)
	case "group":
		return groupExampleSnippet(typeName)
	case "route":
		return routeExampleSnippet(typeName)
	case "traffic":
		if typeName != "" {
			return "", unsupportedExampleType("traffic", typeName, []string{""})
		}
		return trafficExampleSnippet(), nil
	default:
		return "", fmt.Errorf("不支持的 example 类型 %q，支持：global, instance, inbound, outbound, group, route, traffic", kind)
	}
}

func unsupportedExampleType(kind string, typeName string, supported []string) error {
	if len(supported) == 1 && supported[0] == "" {
		return fmt.Errorf("%s example 不支持额外 TYPE %q", kind, typeName)
	}
	return fmt.Errorf("不支持的 %s example TYPE %q，支持：%s", kind, typeName, strings.Join(supported, ", "))
}

func normalizeExampleType(typeName string) string {
	switch strings.ReplaceAll(strings.ReplaceAll(typeName, "-", ""), "_", "") {
	case "ss", "shadowsocks22", "shadowsocks2022":
		return "shadowsocks"
	case "socks":
		return "socks5"
	case "hy2", "hysteria2":
		return "hysteria2"
	case "urltest":
		return "urltest"
	default:
		return typeName
	}
}

func globalExampleSnippet() string {
	return `# Agent 全局配置示例，默认路径为 <base-dir>/config.yaml。
version: 1 # 配置版本，当前只能为 1。
external_host: proxy.example.com # 订阅节点对外暴露的主机名；不要带 scheme、path 或 query。

paths:
  bin: bin # sboxctl 管理的 sing-box 等二进制目录。
  rules: rules # geoip/geosite 等规则资源目录。
  instances: instances # instance YAML/JSON 配置目录。
  runtime: runtime # 运行时状态目录。
  generated: runtime/generated # 渲染后的 sing-box 配置目录，必须位于 runtime 下。
  publish: publish # export/sub bundle 等发布产物目录。
  traffic: traffic # traffic 数据库和配置目录。
  downloads: downloads # 下载缓存目录。
  logs: logs # 预留日志目录。

port_ranges:
  inbound:
    start: 24000 # 自动分配公网 inbound 端口起始值。
    end: 24999 # 自动分配公网 inbound 端口结束值。
  local_socks:
    start: 17000 # 本地 socks5 inbound 端口范围。
    end: 17999
  local_http:
    start: 18000 # 本地 http inbound 端口范围。
    end: 18999
  api:
    start: 10080 # sing-box API 端口范围。
    end: 10099

defaults:
  log_level: info # sing-box 日志级别：trace、debug、info、warn、error。
  api:
    enabled: false # 是否默认启用 sing-box V2Ray API/stats。
    listen: 127.0.0.1:10085 # API 监听地址；非 loopback 监听必须配置 token。
    token: "" # API 鉴权 token，config show 默认会脱敏。
  clash_api:
    enabled: false # 是否启用 Clash API。
    listen: 127.0.0.1:19090
    token: ""
  traffic:
    enabled: true # instance 默认是否采集流量。
    timezone: Asia/Shanghai # IANA 时区名。
    retention_days: 90 # 原始小时数据保留天数。
    daily_min_retention_days: 365 # 日聚合数据最少保留天数。
    monthly_retention_months: 24 # 月聚合数据保留月数。
    timeout_seconds: 5 # 查询 sing-box stats 的超时时间。
    timer:
      hourly: "5 * * * *" # 小时采集 cron。
      daily: "10 0 * * *" # 日聚合 cron。
      monthly: "20 0 1 * *" # 月聚合 cron。

security:
  require_auth_for_public_socks_http: true # 公网 socks5/http inbound 默认要求密码鉴权。
  allow_noauth_public: false # 只有明确放开时才允许公网 noauth。
`
}

func instanceExampleSnippet(typeName string) (string, error) {
	typeName = normalizeExampleType(typeName)
	switch typeName {
	case "", "edge":
		return `# Edge instance 完整示例，可保存为 instances/edge-us.yaml。
name: edge-us # 实例名，必须是安全 basename，不能为 ALL。
enabled: true # false 时不参与默认生命周期操作。
role: edge # 支持 edge、relay、urltest。
labels: [prod, us] # 自定义标签，用于展示和过滤。

api:
  enabled: false # 是否覆盖全局默认并启用 sing-box API。
  listen: 127.0.0.1:10085 # API 监听地址。
  token: "" # 非 loopback 监听必须配置 token。

inbounds:
  - name: vmess-main # inbound 名称，实例内唯一。
    type: vmess # 支持 vmess、shadowsocks、socks5、http。
    listen: 0.0.0.0 # 监听主机，不包含端口。
    port: 24100 # 监听端口，范围 1-65535。
    tag: vmess-vmess-main # sing-box inbound tag；不填默认 <type>-<name>。
    udp: true # 是否启用 UDP。
    users:
      - name: alice # 用户名，vmess/shadowsocks 必填。
        uuid: 11111111-1111-4111-8111-111111111111 # vmess 用户 UUID。
        remark: US VMess # subscription.remark 为空时的订阅展示名。
        tag: edge-us-vmess-main # 订阅节点 tag 覆盖。
    subscription:
      enabled: true # 是否导出到 sboxsub 订阅 input。
      user: alice # 启用订阅时必填，必须引用 users.name。
      server: proxy.example.com # 为空时使用全局 external_host。
      remark: US VMess # 启用订阅时必填，客户端展示名。
      region: US # 可选，两位大写地区码。
  - name: local-socks
    type: socks5
    listen: 127.0.0.1
    port: 17000
    tag: socks5-local-socks
    udp: true
    auth:
      type: noauth # loopback 本地代理可用 noauth；公网监听建议 password。
      username: ""
      password: ""
    subscription:
      enabled: false
      user: ""
      server: ""
      remark: ""
      region: ""
  - name: local-http
    type: http
    listen: 127.0.0.1
    port: 18000
    tag: http-local-http
    udp: false
    auth:
      type: noauth
      username: ""
      password: ""
    subscription:
      enabled: false
      user: ""
      server: ""
      remark: ""
      region: ""

outbounds:
  - name: direct # outbound 名称，也会作为 sing-box tag。
    type: direct # 直连，不需要 server/port。
  - name: block
    type: block # 阻断，不需要 server/port。
  - name: ss-upstream
    type: shadowsocks
    server: ss.example.com # 远端类型必填。
    port: 443
    method: 2022-blake3-aes-256-gcm # Shadowsocks 2022 示例方法。
    password: change-me # shadowsocks 必填。
    tls:
      enabled: false # 当前模型仅支持 TLS 开关。
  - name: vmess-upstream
    type: vmess
    server: vmess.example.com
    port: 443
    uuid: 22222222-2222-4222-8222-222222222222 # vmess 必填。
    tls:
      enabled: true
    network: tcp # vmess 可选；非 tcp 时生成同名 transport type。

groups:
  - name: auto
    type: urltest # 支持 selector、urltest。
    outbounds: [ss-upstream, vmess-upstream, direct] # 引用已定义 outbound。
    url: http://www.gstatic.com/generate_204 # urltest 探测地址。
    interval: 300 # 探测间隔，单位秒。
    tolerance: 50 # 延迟容差，单位毫秒。

route:
  default: auto # 默认 outbound 或 group 名称。
  rules:
    - type: domain_suffix # 支持 domain、domain_suffix、domain_keyword、ip_cidr、geoip、geosite。
      values: [google.com, youtube.com]
      outbound: auto
    - type: ip_cidr
      values: [10.0.0.0/8, 192.168.0.0/16]
      outbound: direct

traffic:
  enabled: true # 是否为该实例采集 traffic。
  scopes: [user, inbound, outbound] # 支持 user、inbound、outbound。
`, nil
	case "relay":
		return `# Relay instance 示例，公网入口使用 Shadowsocks 2022。
name: relay-us
enabled: true
role: relay
labels: [relay, us]
api:
  enabled: false
  listen: 127.0.0.1:10086
  token: ""
inbounds:
  - name: ss-main
    type: shadowsocks
    listen: 0.0.0.0
    port: 24200
    tag: shadowsocks-ss-main
    udp: true
    method: 2022-blake3-aes-256-gcm # 可被 users[].method 覆盖。
    users:
      - name: alice
        password: change-me-32-byte-key # shadowsocks 用户必填。
        method: 2022-blake3-aes-256-gcm # 为空时继承 inbound.method。
        remark: US Shadowsocks
        tag: relay-us-ss-main
    subscription:
      enabled: true
      user: alice
      server: proxy.example.com
      remark: US Shadowsocks
      region: US
outbounds:
  - name: direct
    type: direct
route:
  default: direct
traffic:
  enabled: true
  scopes: [user, inbound, outbound]
`, nil
	case "urltest":
		return `# URLTest instance 示例，通过 group 自动选择延迟更低的 outbound。
name: auto-us
enabled: true
role: urltest
labels: [auto, us]
api:
  enabled: false
  listen: 127.0.0.1:10087
  token: ""
inbounds:
  - name: vmess-main
    type: vmess
    listen: 0.0.0.0
    port: 24300
    tag: vmess-vmess-main
    udp: true
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111
    subscription:
      enabled: true
      user: alice
      server: proxy.example.com
      remark: Auto VMess
      region: US
outbounds:
  - name: proxy-a
    type: shadowsocks
    server: a.example.com
    port: 443
    method: 2022-blake3-aes-256-gcm
    password: change-me-a
  - name: proxy-b
    type: trojan
    server: b.example.com
    port: 443
    password: change-me-b
    tls:
      enabled: true
groups:
  - name: auto
    type: urltest
    outbounds: [proxy-a, proxy-b]
    url: http://www.gstatic.com/generate_204
    interval: 300
    tolerance: 50
route:
  default: auto
traffic:
  enabled: true
  scopes: [user, inbound, outbound]
`, nil
	default:
		return "", unsupportedExampleType("instance", typeName, []string{"edge", "relay", "urltest"})
	}
}

func inboundExampleSnippet(typeName string) (string, error) {
	typeName = normalizeExampleType(typeName)
	switch typeName {
	case "", "all":
		return inboundVmessExample() + "\n" + inboundVLESSExample() + "\n" + inboundAnyTLSExample() + "\n" + inboundShadowsocksExample() + "\n" + inboundSocks5Example() + "\n" + inboundHTTPExample(), nil
	case "vmess":
		return inboundVmessExample(), nil
	case "vless":
		return inboundVLESSExample(), nil
	case "anytls":
		return inboundAnyTLSExample(), nil
	case "shadowsocks":
		return inboundShadowsocksExample(), nil
	case "socks5":
		return inboundSocks5Example(), nil
	case "http":
		return inboundHTTPExample(), nil
	default:
		return "", unsupportedExampleType("inbound", typeName, []string{"vmess", "vless", "anytls", "shadowsocks", "shadowsocks22", "socks5", "http", "all"})
	}
}

func inboundVmessExample() string {
	return `# VMess inbound，可放入 instance.inbounds。
- name: vmess-main # 实例内唯一名称。
  type: vmess # 协议类型。
  listen: 0.0.0.0 # 监听主机，不包含端口。
  port: 24100 # 监听端口。
  tag: vmess-vmess-main # 可选；不填默认 <type>-<name>。
  udp: true # 是否启用 UDP。
  users:
    - name: alice # VMess 用户名。
      uuid: 11111111-1111-4111-8111-111111111111 # VMess 必填 UUID。
      remark: US VMess # subscription.remark 为空时的订阅展示名。
      tag: edge-us-vmess-main # 订阅节点 tag 覆盖。
  subscription:
    enabled: true # 是否导出订阅节点。
    user: alice # 启用时必填，必须引用 users.name。
    server: proxy.example.com # 为空时使用 global.external_host。
    remark: US VMess # 启用时必填。
    region: US # 可选，两位大写地区码。`
}

// inboundVLESSExample 返回 VLESS inbound 示例片段。
func inboundVLESSExample() string {
	return `# VLESS inbound，可放入 instance.inbounds。
- name: vless-main
  type: vless
  listen: 0.0.0.0
  port: 24110
  tag: vless-vless-main
  udp: true
  tls:
    enabled: true # 可按需启用 TLS。
  transport:
    type: ws # 可选；支持 http、ws、quic、grpc、httpupgrade。
    path: /vless
    headers:
      Host: proxy.example.com
  users:
    - name: alice
      uuid: 33333333-3333-4333-8333-333333333333 # VLESS 必填 UUID。
      flow: xtls-rprx-vision # 可选；当前仅支持 xtls-rprx-vision。
      remark: US VLESS
      tag: edge-us-vless-main
  subscription:
    enabled: true
    user: alice
    server: proxy.example.com
    remark: US VLESS
    region: US`
}

// inboundAnyTLSExample 返回 AnyTLS inbound 示例片段。
func inboundAnyTLSExample() string {
	return `# AnyTLS inbound，可放入 instance.inbounds。
- name: anytls-main
  type: anytls
  listen: 0.0.0.0
  port: 24120
  tag: anytls-anytls-main
  udp: true
  tls:
    enabled: true # AnyTLS 必须启用 TLS。
  users:
    - name: alice
      password: change-me # AnyTLS 用户必填密码。
      remark: US AnyTLS
      tag: edge-us-anytls-main
  subscription:
    enabled: true
    user: alice
    server: proxy.example.com
    remark: US AnyTLS
    region: US`
}

func inboundShadowsocksExample() string {
	return `# Shadowsocks inbound，method 可使用 Shadowsocks 2022 方法。
- name: ss-main
  type: shadowsocks # 配置类型仍为 shadowsocks；shadowsocks22 是示例别名。
  listen: 0.0.0.0
  port: 24200
  tag: shadowsocks-ss-main
  udp: true
  method: 2022-blake3-aes-256-gcm # 可被 users[].method 覆盖。
  users:
    - name: alice
      password: change-me-32-byte-key # Shadowsocks 用户必填。
      method: 2022-blake3-aes-256-gcm # 为空时继承 inbound.method。
      remark: US Shadowsocks
      tag: edge-us-ss-main
  subscription:
    enabled: true
    user: alice
    server: proxy.example.com
    remark: US Shadowsocks
    region: US`
}

func inboundSocks5Example() string {
	return `# SOCKS5 inbound，本地监听可 noauth，公网监听建议 password。
- name: local-socks
  type: socks5
  listen: 127.0.0.1
  port: 17000
  tag: socks5-local-socks
  udp: true
  auth:
    type: password # 支持 noauth、password。
    username: alice # type=password 时必填。
    password: change-me # type=password 时必填。
  subscription:
    enabled: false # socks5/http 也可导出订阅，启用时填写下面字段。
    user: alice
    server: proxy.example.com
    remark: Local SOCKS5
    region: US`
}

func inboundHTTPExample() string {
	return `# HTTP inbound，本地监听可 noauth，公网监听建议 password。
- name: local-http
  type: http
  listen: 127.0.0.1
  port: 18000
  tag: http-local-http
  udp: false
  auth:
    type: password # 支持 noauth、password。
    username: alice
    password: change-me
  subscription:
    enabled: false
    user: alice
    server: proxy.example.com
    remark: Local HTTP
    region: US`
}

func outboundExampleSnippet(typeName string) (string, error) {
	typeName = normalizeExampleType(typeName)
	switch typeName {
	case "", "all":
		return outboundDirectExample() + "\n" + outboundBlockExample() + "\n" + outboundShadowsocksExample() + "\n" + outboundVMessExample() + "\n" + outboundVLESSExample() + "\n" + outboundAnyTLSExample() + "\n" + outboundTrojanExample() + "\n" + outboundHysteria2Example() + "\n" + outboundSocks5Example() + "\n" + outboundHTTPExample(), nil
	case "direct":
		return outboundDirectExample(), nil
	case "block":
		return outboundBlockExample(), nil
	case "shadowsocks":
		return outboundShadowsocksExample(), nil
	case "vmess":
		return outboundVMessExample(), nil
	case "vless":
		return outboundVLESSExample(), nil
	case "anytls":
		return outboundAnyTLSExample(), nil
	case "trojan":
		return outboundTrojanExample(), nil
	case "hysteria2":
		return outboundHysteria2Example(), nil
	case "socks5":
		return outboundSocks5Example(), nil
	case "http":
		return outboundHTTPExample(), nil
	default:
		return "", unsupportedExampleType("outbound", typeName, []string{"direct", "block", "shadowsocks", "shadowsocks22", "vmess", "vless", "anytls", "trojan", "hysteria2", "socks5", "http", "all"})
	}
}

func outboundDirectExample() string {
	return `# Direct outbound，直连出站。
- name: direct # outbound 名称，也会作为 sing-box tag。
  type: direct # 不需要 server、port 或认证字段。`
}

func outboundBlockExample() string {
	return `# Block outbound，阻断匹配流量。
- name: block
  type: block # 不需要 server、port 或认证字段。`
}

func outboundShadowsocksExample() string {
	return `# Shadowsocks outbound，含 Shadowsocks 2022 示例方法。
- name: ss-upstream
  type: shadowsocks
  server: ss.example.com # 远端服务器主机名或 IP。
  port: 443 # 远端端口。
  method: 2022-blake3-aes-256-gcm # Shadowsocks 必填。
  password: change-me # Shadowsocks 必填。
  tls:
    enabled: false # 当前模型仅支持 TLS 开关。`
}

func outboundVMessExample() string {
	return `# VMess outbound。
- name: vmess-upstream
  type: vmess
  server: vmess.example.com
  port: 443
  uuid: 22222222-2222-4222-8222-222222222222 # VMess 必填。
  tls:
    enabled: true # 是否启用 TLS。
  network: tcp # 可选；VMess 底层网络，仅支持 tcp、udp。
  transport:
    type: ws # 可选；支持 http、ws、quic、grpc、httpupgrade。
    path: /vmess
    headers:
      Host: vmess.example.com`
}

// outboundVLESSExample 返回 VLESS outbound 示例片段。
func outboundVLESSExample() string {
	return `# VLESS outbound。
- name: vless-upstream
  type: vless
  server: vless.example.com
  port: 443
  uuid: 33333333-3333-4333-8333-333333333333 # VLESS 必填。
  flow: xtls-rprx-vision # 可选；当前仅支持 xtls-rprx-vision。
  tls:
    enabled: true # 是否启用 TLS。
  transport:
    type: httpupgrade # 可选；支持 http、ws、quic、grpc、httpupgrade。
    host: vless.example.com
    path: /upgrade`
}

// outboundAnyTLSExample 返回 AnyTLS outbound 示例片段。
func outboundAnyTLSExample() string {
	return `# AnyTLS outbound。
- name: anytls-upstream
  type: anytls
  server: anytls.example.com
  port: 443
  password: change-me # AnyTLS 必填。
  tls:
    enabled: true # AnyTLS 必须启用 TLS。`
}

func outboundTrojanExample() string {
	return `# Trojan outbound。
- name: trojan-upstream
  type: trojan
  server: trojan.example.com
  port: 443
  password: change-me # Trojan 必填。
  tls:
    enabled: true # Trojan 通常需要 TLS。`
}

func outboundHysteria2Example() string {
	return `# Hysteria2 outbound。
- name: hy2-upstream
  type: hysteria2
  server: hy2.example.com
  port: 443
  password: change-me # Hysteria2 必填。
  tls:
    enabled: true`
}

func outboundSocks5Example() string {
	return `# SOCKS5 outbound。
- name: socks5-upstream
  type: socks5
  server: socks.example.com
  port: 1080
  auth:
    type: password # 可为空、noauth 或 password。
    username: alice # type=password 时必填。
    password: change-me # type=password 时必填。
  tls:
    enabled: false`
}

func outboundHTTPExample() string {
	return `# HTTP outbound。
- name: http-upstream
  type: http
  server: http-proxy.example.com
  port: 8080
  auth:
    type: password # 可为空、noauth 或 password。
    username: alice
    password: change-me
  tls:
    enabled: false`
}

func groupExampleSnippet(typeName string) (string, error) {
	typeName = normalizeExampleType(typeName)
	switch typeName {
	case "", "all":
		return groupSelectorExample() + "\n" + groupURLTestExample(), nil
	case "selector":
		return groupSelectorExample(), nil
	case "urltest":
		return groupURLTestExample(), nil
	default:
		return "", unsupportedExampleType("group", typeName, []string{"selector", "urltest", "all"})
	}
}

func groupSelectorExample() string {
	return `# Selector group，客户端或运行时可在多个 outbound 间选择。
- name: manual
  type: selector
  outbounds: [ss-upstream, vmess-upstream, direct] # 必须引用已定义 outbound。`
}

func groupURLTestExample() string {
	return `# URLTest group，按探测延迟自动选择 outbound。
- name: auto
  type: urltest
  outbounds: [ss-upstream, vmess-upstream, direct]
  url: http://www.gstatic.com/generate_204 # 探测 URL，默认同此值。
  interval: 300 # 探测间隔，单位秒。
  tolerance: 50 # 延迟容差，单位毫秒。`
}

func routeExampleSnippet(typeName string) (string, error) {
	typeName = normalizeExampleType(typeName)
	switch typeName {
	case "", "all":
		return `# Route 示例，可放入 instance.route。
default: auto # 默认 outbound 或 group 名称。
rules:
  - type: domain # 完整域名匹配。
    values: [example.com]
    outbound: auto
  - type: domain_suffix # 域名后缀匹配。
    values: [google.com, youtube.com]
    outbound: auto
  - type: domain_keyword # 域名关键字匹配。
    values: [netflix]
    outbound: auto
  - type: ip_cidr # IP CIDR 匹配。
    values: [10.0.0.0/8, 192.168.0.0/16]
    outbound: direct
  - type: geoip # geoip 规则匹配。
    values: [cn, private]
    outbound: direct
  - type: geosite # geosite 规则匹配。
    values: [category-ads-all]
    outbound: block
`, nil
	case "domain", "domain_suffix", "domain_keyword", "ip_cidr", "geoip", "geosite":
		return fmt.Sprintf(`# Route %s 规则示例。
default: auto
rules:
  - type: %s
    values: [%s]
    outbound: auto
`, typeName, typeName, routeExampleValue(typeName)), nil
	default:
		return "", unsupportedExampleType("route", typeName, []string{"domain", "domain_suffix", "domain_keyword", "ip_cidr", "geoip", "geosite", "all"})
	}
}

func routeExampleValue(typeName string) string {
	switch typeName {
	case "domain":
		return "example.com"
	case "domain_suffix":
		return "google.com"
	case "domain_keyword":
		return "netflix"
	case "ip_cidr":
		return "10.0.0.0/8"
	case "geoip":
		return "cn"
	case "geosite":
		return "category-ads-all"
	default:
		return "example.com"
	}
}

func trafficExampleSnippet() string {
	return `# 独立 traffic config 示例，默认路径为 <base-dir>/traffic/config.yaml。
version: 1 # 配置版本，当前只能为 1。
enabled: true # 是否启用采集。
timezone: Asia/Shanghai # IANA 时区名。
retention_days: 90 # 原始小时数据保留天数。
daily_min_retention_days: 365 # 日聚合数据最少保留天数。
monthly_retention_months: 24 # 月聚合数据保留月数。
timeout_seconds: 5 # 查询 sing-box stats 的超时时间。
timer:
  hourly: "5 * * * *" # 小时采集 cron。
  daily: "10 0 * * *" # 日聚合 cron。
  monthly: "20 0 1 * *" # 月聚合 cron。

# Instance 内的 traffic 片段只需要：
# traffic:
#   enabled: true
#   scopes: [user, inbound, outbound]
`
}
