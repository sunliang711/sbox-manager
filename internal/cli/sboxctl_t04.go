package cli

import (
	"context"
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
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "初始化完成: %s\n", options.baseDir)
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
			if err := newResourceInstaller().Run(cmd.Context(), set.Global, installer.Options{Operation: installer.OperationInstall, Resource: installer.ResourceAll}); err != nil {
				return err
			}
			manager, err := newSboxctlServiceManager(options)
			if err != nil {
				return err
			}
			if err := manager.Install(cmd.Context(), set.BaseDir, set.Global, set.Instances, ""); err != nil {
				return err
			}
			if start {
				return runSboxctlRuntimeLifecycle(cmd, "start", nil)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "setup 完成")
			return err
		},
	}
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
			text, err := exampleSnippet(kind)
			if err != nil {
				return err
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
		Short: "管理实例 systemd unit 或 launchd plist",
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
			options, set, manager, err := loadServiceCommandContext(cmd)
			if err != nil {
				return err
			}
			if err := manager.Install(cmd.Context(), set.BaseDir, set.Global, set.Instances, optionalArg(args)); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "service install 完成: %s\n", options.serviceManager)
			return err
		},
	}
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
			set, err := config.LoadAgentConfigSet(options.baseDir)
			if err != nil {
				return err
			}
			err = newResourceInstaller().Run(cmd.Context(), set.Global, installer.Options{
				Operation:     use,
				Resource:      args[0],
				Version:       version,
				Source:        source,
				SHA256:        sha256Text,
				ArchiveMember: member,
				Purge:         purge,
			})
			if err != nil {
				return err
			}
			if use == installer.OperationUninstall && purge {
				manager, err := newSboxctlServiceManager(options)
				if err != nil {
					return err
				}
				if err := manager.Uninstall(cmd.Context(), set.Instances, ""); err != nil {
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

func exampleSnippet(kind string) (string, error) {
	switch kind {
	case "global":
		return `version: 1
external_host: proxy.example.com
`, nil
	case "instance":
		return `name: edge-us
api:
  enabled: false
inbounds:
  - name: vmess-main
    type: vmess
    listen: 0.0.0.0
    port: 24100
    users:
      - name: alice
        uuid: 11111111-1111-4111-8111-111111111111
outbounds:
  - name: direct
    type: direct
route:
  default: direct
`, nil
	case "inbound":
		return `- name: vmess-main
  type: vmess
  listen: 0.0.0.0
  port: 24100
  users:
    - name: alice
      uuid: 11111111-1111-4111-8111-111111111111
`, nil
	case "outbound":
		return `- name: direct
  type: direct
`, nil
	case "group":
		return `- name: auto
  type: urltest
  outbounds: [direct]
  url: http://www.gstatic.com/generate_204
  interval: 300
`, nil
	case "route":
		return `default: direct
rules:
  - type: domain_suffix
    values: [example.com]
    outbound: direct
`, nil
	case "traffic":
		return `enabled: true
scopes: [user, inbound, outbound]
`, nil
	default:
		return "", fmt.Errorf("不支持的 example 类型 %q", kind)
	}
}
