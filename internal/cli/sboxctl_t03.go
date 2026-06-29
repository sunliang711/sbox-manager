package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
	"github.com/sunliang711/sbox-manager/internal/generator/singbox"
	runtimeplan "github.com/sunliang711/sbox-manager/internal/runtime"
	"github.com/sunliang711/sbox-manager/internal/service"
)

var (
	newRuntimeConfigChecker = func(_ *rootOptions, global domain.GlobalConfig) runtimeplan.ConfigChecker {
		if binary := filepath.Join(global.Paths.Bin, "sing-box"); binary != "" {
			if info, err := os.Stat(binary); err == nil && !info.IsDir() {
				return runtimeplan.CommandConfigChecker{Binary: binary}
			}
		}
		return runtimeplan.CommandConfigChecker{}
	}
	newRuntimeServiceManager = func(options *rootOptions, global domain.GlobalConfig) (runtimeplan.ServiceManager, error) {
		return newSboxctlServiceManager(options)
	}
	newSboxctlServiceManager = func(options *rootOptions) (*service.Manager, error) {
		return service.NewManager(service.Options{Kind: options.serviceManager})
	}
	cliNow = time.Now
)

// newSboxctlValidateCommand 创建 T03 已实现的 validate 命令。
func newSboxctlValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate [TARGET]",
		Short: "校验 agent 配置",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				if _, ok := set.FindInstance(args[0]); !ok {
					return fmt.Errorf("instance %q 不存在", args[0])
				}
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "配置校验通过")
			return err
		},
	}
	cmd.Flags().Bool("skip-system-ports", false, "跳过系统端口检查")
	return cmd
}

// newSboxctlCheckCommand 创建 T03 已实现的 check 命令。
func newSboxctlCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check [TARGET]",
		Short: "检查生成计划和服务变更",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			target := optionalArg(args)
			plan, err := runtimeplan.BuildPlan(set.Global, set.Instances, target)
			if err != nil {
				return err
			}
			return writePlanPreview(cmd, plan)
		},
	}
	cmd.Flags().Bool("skip-system-ports", false, "跳过系统端口检查")
	return cmd
}

// newSboxctlRenderCommand 创建渲染命令组。
func newSboxctlRenderCommand() *cobra.Command {
	render := &cobra.Command{
		Use:   "render",
		Short: "渲染模型、sing-box 配置或订阅 bundle",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	render.PersistentFlags().Bool("skip-system-ports", false, "跳过系统端口检查")
	render.AddCommand(
		newSboxctlRenderModelCommand(),
		newSboxctlRenderSingBoxCommand(),
		newSboxctlRenderSubCommand(),
	)
	mustCommand(render, "sub").Flags().String("input-dir", "", "订阅 input 目录")
	return render
}

// newSboxctlRenderModelCommand 创建 render model 命令。
func newSboxctlRenderModelCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "model",
		Short: "渲染完整配置模型",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			data, err := singbox.MarshalStable(struct {
				Global    domain.GlobalConfig `json:"global"`
				Instances []domain.Instance   `json:"instances"`
			}{
				Global:    set.Global,
				Instances: set.Instances,
			})
			if err != nil {
				return err
			}
			return writeOutput(cmd, data)
		},
	}
}

// newSboxctlRenderSingBoxCommand 创建 render sing-box 命令。
func newSboxctlRenderSingBoxCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sing-box INSTANCE",
		Short: "渲染指定实例的 sing-box 配置",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			instance, ok := set.FindInstance(args[0])
			if !ok {
				return fmt.Errorf("instance %q 不存在", args[0])
			}
			data, err := singbox.GenerateWithInstances(set.Global, set.Instances, instance)
			if err != nil {
				return err
			}
			return writeOutput(cmd, data)
		},
	}
}

// newSboxctlRenderSubCommand 创建 render sub 命令。
func newSboxctlRenderSubCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sub",
		Short: "渲染订阅 input",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			instances, err := set.TargetInstances("")
			if err != nil {
				return err
			}
			inputs, err := singbox.BuildSubscriptionInputs(set.Global, instances, cliNow())
			if err != nil {
				return err
			}
			data, err := singbox.MarshalStable(inputs)
			if err != nil {
				return err
			}
			return writeOutput(cmd, data)
		},
	}
}

// newSboxctlExportConfigCommand 创建 T03 最小可用 export-config 命令。
func newSboxctlExportConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "export-config clash|premium-clash|surge|sing-box USER",
		Short:     "导出指定用户订阅配置",
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{"clash", "premium-clash", "surge", "sing-box"},
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			instances, err := set.TargetInstances("")
			if err != nil {
				return err
			}
			inputs, err := singbox.BuildSubscriptionInputs(set.Global, instances, cliNow())
			if err != nil {
				return err
			}
			data, err := singbox.RenderUserConfig(args[0], args[1], inputs)
			if err != nil {
				return err
			}
			return writeOutput(cmd, data)
		},
	}
	return cmd
}

// newSboxctlLifecycleCommand 创建 T03 已实现的 start/restart 命令。
func newSboxctlLifecycleCommand(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " [TARGET]",
		Short: lifecycleShort(action),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			target := optionalArg(args)
			plan, err := runtimeplan.BuildPlan(set.Global, set.Instances, target)
			if err != nil {
				return err
			}
			checker := newRuntimeConfigChecker(options, set.Global)
			manager, err := newRuntimeServiceManager(options, set.Global)
			if err != nil {
				return err
			}
			var result *runtimeplan.ApplyResult
			switch action {
			case "start":
				result, err = runtimeplan.Start(cmd.Context(), plan, checker, manager, runtimeplan.DefaultClock)
			case "restart":
				result, err = runtimeplan.Restart(cmd.Context(), plan, checker, manager, runtimeplan.DefaultClock)
			default:
				return fmt.Errorf("不支持的生命周期动作 %q", action)
			}
			if err != nil {
				return err
			}
			if err := writePlanPreview(cmd, plan); err != nil {
				return err
			}
			if result != nil && result.Changed {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s 完成，runtime 已更新\n", action)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s 完成，runtime 无变化\n", action)
			return err
		},
	}
}

// lifecycleShort 返回生命周期命令说明。
func lifecycleShort(action string) string {
	if action == "restart" {
		return "重新生成 runtime 并重启实例服务"
	}
	return "生成 runtime 并启动实例服务"
}

// loadAgentSetFromCommand 根据 CLI 全局参数加载 agent 配置集合。
func loadAgentSetFromCommand(cmd *cobra.Command) (*config.AgentConfigSet, error) {
	options, err := getRootOptions(cmd)
	if err != nil {
		return nil, err
	}
	return config.LoadAgentConfigSet(options.baseDir)
}

// writePlanPreview 输出 runtime plan diff 预览。
func writePlanPreview(cmd *cobra.Command, plan *runtimeplan.Plan) error {
	if plan == nil || len(plan.Changes) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "no-change")
		return err
	}
	for _, change := range plan.Changes {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s %s\n", change.Action, change.Instance, change.RelativePath, change.Service); err != nil {
			return err
		}
	}
	return nil
}

// writeOutput 将命令结果写到 stdout。
func writeOutput(cmd *cobra.Command, data []byte) error {
	_, err := cmd.OutOrStdout().Write(data)
	return err
}

// optionalArg 返回可选位置参数。
func optionalArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}
