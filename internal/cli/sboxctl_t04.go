package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/configtemplate"
	"github.com/sunliang711/sbox-manager/internal/domain"
	installer "github.com/sunliang711/sbox-manager/internal/install"
	instancemgr "github.com/sunliang711/sbox-manager/internal/instance"
	runtimeplan "github.com/sunliang711/sbox-manager/internal/runtime"
	"github.com/sunliang711/sbox-manager/internal/service"
)

type resourceInstallerRunner interface {
	Run(ctx context.Context, global domain.GlobalConfig, options installer.Options) error
}

var newResourceInstaller = func() resourceInstallerRunner {
	return installer.NewInstaller()
}

// newSboxctlSetupCommand 创建 setup 命令。
func newSboxctlSetupCommand() *cobra.Command {
	setup := &cobra.Command{
		Use:   "setup",
		Short: "Prepare local config, services, and runtime resources",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSboxctlSetupAll(cmd)
		},
	}
	setup.AddCommand(
		&cobra.Command{
			Use:   "local",
			Short: "Prepare local config directories and install service files",
			Args:  cobra.NoArgs,
			RunE:  runSboxctlSetupLocal,
		},
		&cobra.Command{
			Use:   "binary",
			Short: "Download and install sing-box, geosite, and geoip",
			Args:  cobra.NoArgs,
			RunE:  runSboxctlSetupBinary,
		},
		&cobra.Command{
			Use:   "all",
			Short: "Run both local and binary setup stages",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runSboxctlSetupAll(cmd)
			},
		},
	)
	return setup
}

// runSboxctlSetupLocal 准备本机目录、默认配置、服务文件和 traffic timer。
func runSboxctlSetupLocal(cmd *cobra.Command, args []string) error {
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	externalHost, _ := cmd.Flags().GetString("external-host")
	force, _ := cmd.Flags().GetBool("force")
	if err := instancemgr.Init(options.baseDir, instancemgr.InitOptions{ExternalHost: externalHost, Force: force, AllowExisting: true}); err != nil {
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
	if err := installSboxctlServiceFiles(cmd, manager, set, ""); err != nil {
		return err
	}
	return writeStatus(cmd, outputStatusOK, "Local setup completed.", outputKV("Base dir", options.baseDir))
}

// runSboxctlSetupBinary 下载并安装 agent 运行所需资源。
func runSboxctlSetupBinary(cmd *cobra.Command, args []string) error {
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	set, err := loadSetupBinaryConfigSet(options.baseDir)
	if err != nil {
		return err
	}
	progress := newInstallProgressWriter(cmd)
	err = newResourceInstaller().Run(cmd.Context(), set.Global, installer.Options{
		Operation: installer.OperationInstall,
		Resource:  installer.ResourceAll,
		Progress:  progress.Write,
	})
	progress.Finish()
	if err != nil {
		return err
	}
	return writeStatus(cmd, outputStatusOK, "Binary resources are ready.")
}

// runSboxctlSetupAll 依次执行 local 和 binary 阶段。
func runSboxctlSetupAll(cmd *cobra.Command) error {
	if err := runSboxctlSetupLocal(cmd, nil); err != nil {
		return err
	}
	if err := runSboxctlSetupBinary(cmd, nil); err != nil {
		return err
	}
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	if err := writeStatus(cmd, outputStatusOK, "Agent environment setup completed."); err != nil {
		return err
	}
	return writeNextSteps(cmd, sboxctlSetupNextSteps(options.baseDir)...)
}

// loadSetupBinaryConfigSet 加载 binary 阶段所需路径配置；缺少配置时使用默认 base-dir 路径。
func loadSetupBinaryConfigSet(baseDir string) (*config.AgentConfigSet, error) {
	set, err := config.LoadAgentConfigSet(baseDir)
	if err == nil {
		return set, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return defaultAgentConfigSetForPurgedBaseDir(baseDir)
}

// sboxctlSetupNextSteps 返回 agent setup 后的下一步提示。
func sboxctlSetupNextSteps(baseDir string) []string {
	return []string{
		fmt.Sprintf("Create an instance config: sboxctl --base-dir %s add edge-us --template edge", baseDir),
		fmt.Sprintf("Start enabled instances: sudo sboxctl --base-dir %s start", baseDir),
		fmt.Sprintf("Check the environment: sboxctl --base-dir %s doctor", baseDir),
	}
}

// newSboxctlConfigCommand 创建 agent 配置命令。
func newSboxctlConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config [INSTANCE]",
		Short: "Edit or check agent config",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check-only")
			editor, _ := cmd.Flags().GetString("editor")
			if checkOnly {
				if err := checkConfigCommand(cmd, args); err != nil {
					return err
				}
				return writeStatus(cmd, outputStatusOK, "Configuration validation passed.")
			}
			return editConfigCommand(cmd, args, editor)
		},
	}
}

// newSboxctlExampleCommand 创建 example 命令。
func newSboxctlExampleCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "example [global|instance|inbound|outbound|group|route|traffic] [TYPE]",
		Short: "Print configuration example snippets",
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
		Short: "Add a sing-box instance config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			template, _ := cmd.Flags().GetString("template")
			fromFile, _ := cmd.Flags().GetString("from-file")
			members, _ := cmd.Flags().GetString("members")
			allocate, _ := cmd.Flags().GetBool("allocate-ports")
			keep, _ := cmd.Flags().GetBool("keep-template-ports")
			edit, _ := cmd.Flags().GetBool("edit")
			noEdit, _ := cmd.Flags().GetBool("no-edit")
			editor, _ := cmd.Flags().GetString("editor")
			created, err := instancemgr.Add(options.baseDir, instancemgr.AddOptions{
				Name:              args[0],
				Template:          template,
				FromFile:          fromFile,
				Members:           splitCommaList(members),
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
			return writeStatus(cmd, outputStatusOK, "Instance created.", outputKV("Name", created.Name))
		},
	}
}

// splitCommaList 将逗号分隔参数解析为去空白列表。
func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			items = append(items, part)
		}
	}
	return items
}

// writeInstanceList 将 instance 摘要输出为固定列的状态表。
func writeInstanceList(cmd *cobra.Command, options *rootOptions, set *config.AgentConfigSet) error {
	instances := append([]domain.Instance(nil), set.Instances...)
	sort.SliceStable(instances, func(i int, j int) bool {
		return instances[i].Name < instances[j].Name
	})
	manager, err := newSboxctlServiceManager(options)
	if err != nil {
		return err
	}
	rows := [][]string{
		{"Name", "Role", "Enabled", "Running", "Generated", "Ports"},
		{"------", "----", "-------", "-------", "---------", "------------------------------------------------"},
	}
	for _, instance := range instances {
		domain.ApplyInstanceDefaults(&instance)
		rows = append(rows, []string{
			instance.Name,
			instance.Role,
			yesNo(instance.Enabled),
			instanceListRunningStatus(cmd, manager, instance.Name),
			instanceListGeneratedStatus(set.Global, instance.Name),
			instanceListPorts(instance),
		})
	}
	return writeRows(cmd, rows)
}

// instanceListRunningStatus 查询 instance 对应服务的运行态，查询失败时保持列表可读。
func instanceListRunningStatus(cmd *cobra.Command, manager *service.Manager, name string) string {
	serviceName := service.ServiceNameForKind(manager.Kind(), name)
	running, err := manager.IsRunning(cmd.Context(), serviceName)
	if err != nil {
		return "unknown"
	}
	return yesNo(running)
}

// instanceListGeneratedStatus 判断 instance 的 sing-box 运行配置是否已生成。
func instanceListGeneratedStatus(global domain.GlobalConfig, name string) string {
	path := filepath.Join(global.Paths.Generated, "sing-box", name+".json")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "no"
		}
		return "unknown"
	}
	return yesNo(!info.IsDir())
}

// instanceListPorts 汇总 instance 暴露的 API 与 inbound 端口。
func instanceListPorts(instance domain.Instance) string {
	ports := make([]string, 0, len(instance.Inbounds)+1)
	if instance.API.Enabled {
		_, port, err := net.SplitHostPort(instance.API.Listen)
		if err == nil {
			ports = append(ports, "api="+port)
		} else {
			ports = append(ports, "api=?")
		}
	}
	for _, inbound := range instance.Inbounds {
		if inbound.Port <= 0 {
			continue
		}
		name := inbound.Name
		if name == "" {
			name = inbound.Type
		}
		if name == "" {
			name = "inbound"
		}
		ports = append(ports, fmt.Sprintf("%s=%d", name, inbound.Port))
	}
	if len(ports) == 0 {
		return "-"
	}
	return strings.Join(ports, ", ")
}

// yesNo 将布尔状态转成列表中更易读的英文状态。
func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

// newSboxctlListCommand 创建 list 命令。
func newSboxctlListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List instances managed by the agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			return writeInstanceList(cmd, options, set)
		},
	}
}

// newSboxctlCloneCommand 创建 clone 命令。
func newSboxctlCloneCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clone SOURCE TARGET",
		Short: "Clone an instance config",
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
			return writeStatus(cmd, outputStatusOK, "Instance cloned.", outputKV("Source", args[0]), outputKV("Target", cloned.Name))
		},
	}
}

// newSboxctlMemberCommand 创建 group member 管理命令。
func newSboxctlMemberCommand() *cobra.Command {
	member := &cobra.Command{
		Use:   "member",
		Short: "Manage selector or urltest group members",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	member.AddCommand(
		&cobra.Command{
			Use:   "list INSTANCE GROUP",
			Short: "List group members",
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
				rows := make([][]string, 0, len(members))
				for _, member := range members {
					rows = append(rows, []string{member})
				}
				return writeTable(cmd, []string{"MEMBER"}, rows)
			},
		},
		memberMutationCommand("add", "Add a group member", instancemgr.MemberAdd),
		memberMutationCommand("remove", "Remove a group member", instancemgr.MemberRemove),
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
			return writeStatus(cmd, outputStatusOK, "Group member updated.",
				outputKV("Action", name),
				outputKV("Instance", args[0]),
				outputKV("Group", args[1]),
				outputKV("Member", args[2]),
			)
		},
	}
}

// newSboxctlRemoveCommand 创建 remove 命令。
func newSboxctlRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove an instance config",
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
			return writeStatus(cmd, outputStatusOK, "Instance removed.", outputKV("Name", args[0]))
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
		Short: "Manage instance systemd template unit or launchd plist",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	serviceCommand.AddCommand(
		newSboxctlServiceInstallCommand(),
		newSboxctlServiceUninstallCommand(),
		serviceSubActionCommand("start", "Start instance services"),
		serviceSubActionCommand("stop", "Stop instance services"),
		serviceSubActionCommand("restart", "Restart instance services"),
		serviceSubActionCommand("status", "Show instance service status"),
		serviceSubActionCommand("logs", "Show instance service logs"),
		serviceSubActionCommand("log", "Show instance service logs"),
		serviceSubActionCommand("enable", "Enable instance services"),
		serviceSubActionCommand("disable", "Disable instance services"),
	)
	mustCommand(serviceCommand, "logs").Flags().BoolP("follow", "f", false, "follow logs")
	mustCommand(serviceCommand, "log").Flags().BoolP("follow", "f", false, "follow logs")
	return serviceCommand
}

func newSboxctlServiceInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install [TARGET]",
		Short: "Install instance service files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, set, manager, err := loadServiceCommandContext(cmd)
			if err != nil {
				return err
			}
			if err := installSboxctlServiceFiles(cmd, manager, set, optionalArg(args)); err != nil {
				return err
			}
			return writeStatus(cmd, outputStatusOK, "Instance service files installed.", outputKV("Service manager", manager.Kind()))
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
		return fmt.Errorf("resolve sboxctl path: %w", err)
	}
	if err := manager.InstallTrafficTimers(cmd.Context(), set.BaseDir, set.Global.Paths.Traffic, set.Global.Paths.Logs, binary); err != nil {
		return err
	}
	if _, err := manager.RunTrafficTimers(cmd.Context(), "enable", false); err != nil {
		return err
	}
	return writeStatus(cmd, outputStatusOK, "Traffic timers enabled.")
}

func newSboxctlServiceUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall [TARGET]",
		Short: "Uninstall instance service files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, set, manager, err := loadServiceCommandContext(cmd)
			if err != nil {
				return err
			}
			if err := manager.Uninstall(cmd.Context(), set.Instances, optionalArg(args)); err != nil {
				return err
			}
			return writeStatus(cmd, outputStatusOK, "Instance service files uninstalled.")
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
		return writeStatus(cmd, outputStatusInfo, "No target services matched.", outputKV("Action", action))
	}
	for _, result := range results {
		if len(result.Output) > 0 {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Output: %s\n%s", result.Service, result.Output); err != nil {
				return err
			}
			if !strings.HasSuffix(string(result.Output), "\n") {
				if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
					return err
				}
			}
			continue
		}
		if err := writeStatus(cmd, outputStatusOK, "Service action completed.", outputKV("Action", action), outputKV("Service", result.Service)); err != nil {
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
			progress := newInstallProgressWriter(cmd)
			err = newResourceInstaller().Run(cmd.Context(), set.Global, installer.Options{
				Operation:     use,
				Resource:      args[0],
				Version:       version,
				Source:        source,
				SHA256:        sha256Text,
				ArchiveMember: member,
				Purge:         purge,
				Progress:      progress.Write,
			})
			progress.Finish()
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
			return writeStatus(cmd, outputStatusOK, "Resource operation completed.",
				outputKV("Operation", use),
				outputKV("Resource", args[0]),
			)
		},
	}
	cmd.Flags().String("version", "", "target version")
	cmd.Flags().String("source", "", "download source")
	cmd.Flags().String("sha256", "", "sha256 checksum")
	cmd.Flags().String("archive-member", "", "archive member name")
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
		return nil, fmt.Errorf("clean base-dir %s: %w", baseDir, err)
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
		return fmt.Errorf("refuse to clean dangerous base-dir %s", cleaned)
	}
	exists, err := pathExists(cleaned)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if err := os.RemoveAll(cleaned); err != nil {
		return fmt.Errorf("clean agent base-dir %s: %w", cleaned, err)
	}
	return nil
}

// pathExists 判断 CLI 清理路径是否存在，路径不存在时不视为错误。
func pathExists(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("check path %s: %w", path, err)
	}
	return true, nil
}

func runSboxctlRuntimeLifecycle(cmd *cobra.Command, action string, args []string) error {
	lifecycle := newSboxctlLifecycleCommand(action)
	lifecycle.SetOut(cmd.OutOrStdout())
	lifecycle.SetErr(cmd.ErrOrStderr())
	if args == nil {
		args = []string{}
	}
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
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if err := os.WriteFile(draft, data, 0600); err != nil {
		return fmt.Errorf("write draft file %s: %w", draft, err)
	}
	defer os.Remove(draft)
	if err := instancemgr.EditFileWithCommand(draft, editor); err != nil {
		return err
	}
	draftData, err := os.ReadFile(draft)
	if err != nil {
		return fmt.Errorf("read draft file %s: %w", draft, err)
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
	if bytes.Equal(data, draftData) {
		return writeStatus(cmd, outputStatusInfo, "Configuration unchanged.", outputKV("File", path))
	}
	if err := os.Rename(draft, path); err != nil {
		return fmt.Errorf("replace config file %s: %w", path, err)
	}
	if err := writeStatus(cmd, outputStatusOK, "Configuration updated.", outputKV("File", path)); err != nil {
		return err
	}
	if len(args) == 1 {
		restarted, err := restartEditedInstanceIfRunning(cmd, options, args[0])
		if err != nil {
			return err
		}
		if restarted {
			return writeStatus(cmd, outputStatusOK, "Running instance restarted automatically.", outputKV("Instance", args[0]))
		}
		return writeStatus(cmd, outputStatusInfo, "Instance is not running; automatic restart skipped.", outputKV("Instance", args[0]))
	}
	return nil
}

// restartEditedInstanceIfRunning 在实例配置变更后复用 lifecycle restart 刷新 runtime 并重启运行中服务。
func restartEditedInstanceIfRunning(cmd *cobra.Command, options *rootOptions, name string) (bool, error) {
	manager, err := newSboxctlServiceManager(options)
	if err != nil {
		return false, err
	}
	serviceName := service.ServiceNameForKind(manager.Kind(), name)
	running, err := manager.IsRunning(cmd.Context(), serviceName)
	if err != nil {
		return false, err
	}
	if !running {
		return false, nil
	}
	set, err := config.LoadAgentConfigSet(options.baseDir)
	if err != nil {
		return false, err
	}
	plan, err := runtimeplan.BuildPlan(set.Global, set.Instances, name)
	if err != nil {
		return false, err
	}
	checker := newRuntimeConfigChecker(options, set.Global)
	runtimeManager, err := newRuntimeServiceManager(options, set.Global)
	if err != nil {
		return false, err
	}
	_, err = runtimeplan.Restart(cmd.Context(), plan, checker, runtimeManager, runtimeplan.DefaultClock)
	if err != nil {
		return false, err
	}
	return true, nil
}

func validateInstanceDraft(path string, extension string, global domain.GlobalConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read draft file %s: %w", path, err)
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
		return fmt.Errorf("read config file %s: %w", path, err)
	}
	if err := os.WriteFile(draft, data, 0600); err != nil {
		return fmt.Errorf("write draft file %s: %w", draft, err)
	}
	defer os.Remove(draft)
	if err := instancemgr.EditFileWithCommand(draft, editor); err != nil {
		return err
	}
	if err := validateInstanceDraft(draft, filepath.Ext(path), set.Global); err != nil {
		return err
	}
	if err := os.Rename(draft, path); err != nil {
		return fmt.Errorf("replace config file %s: %w", path, err)
	}
	return nil
}

func draftPath(path string) string {
	return path + ".draft"
}

func serviceActionShort(action string) string {
	switch action {
	case "stop":
		return "Stop instance services"
	case "status":
		return "Show instance service status"
	case "logs":
		return "Show instance service logs"
	case "enable":
		return "Enable instance services"
	case "disable":
		return "Disable instance services"
	default:
		return "Manage instance services"
	}
}

type installProgressWriter struct {
	cmd        *cobra.Command
	inProgress bool
}

// newInstallProgressWriter 创建安装进度输出器，下载进度会覆盖当前行展示。
func newInstallProgressWriter(cmd *cobra.Command) *installProgressWriter {
	return &installProgressWriter{cmd: cmd}
}

// Write 将资源安装器的英文进度日志输出到 stderr。
func (w *installProgressWriter) Write(message string) {
	if strings.HasPrefix(message, "download: progress ") {
		_, _ = fmt.Fprintf(w.cmd.ErrOrStderr(), "\r\033[2K%s", message)
		w.inProgress = true
		return
	}
	w.Finish()
	_, _ = fmt.Fprintln(w.cmd.ErrOrStderr(), message)
}

// Finish 在下载进度覆盖行之后补换行，避免后续输出粘在同一行。
func (w *installProgressWriter) Finish() {
	if !w.inProgress {
		return
	}
	_, _ = fmt.Fprintln(w.cmd.ErrOrStderr())
	w.inProgress = false
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
		return configtemplate.RenderInstanceExample(typeName, configtemplate.DefaultContext())
	case "inbound":
		return configtemplate.RenderExample("inbound", typeName, configtemplate.DefaultContext())
	case "outbound":
		return configtemplate.RenderExample("outbound", typeName, configtemplate.DefaultContext())
	case "group":
		return configtemplate.RenderExample("group", typeName, configtemplate.DefaultContext())
	case "route":
		return configtemplate.RenderExample("route", typeName, configtemplate.DefaultContext())
	case "traffic":
		if typeName != "" {
			return "", unsupportedExampleType("traffic", typeName, []string{""})
		}
		return trafficExampleSnippet(), nil
	default:
		return "", fmt.Errorf("unsupported example kind %q; supported values: global, instance, inbound, outbound, group, route, traffic", kind)
	}
}

func unsupportedExampleType(kind string, typeName string, supported []string) error {
	if len(supported) == 1 && supported[0] == "" {
		return fmt.Errorf("%s example does not accept extra TYPE %q", kind, typeName)
	}
	return fmt.Errorf("unsupported %s example TYPE %q; supported values: %s", kind, typeName, strings.Join(supported, ", "))
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
