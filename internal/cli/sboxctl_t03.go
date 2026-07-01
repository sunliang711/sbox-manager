package cli

import (
	"errors"
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
		Short: "Validate agent configuration",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				instance, ok := set.FindInstance(args[0])
				if !ok {
					return fmt.Errorf("instance %q does not exist", args[0])
				}
				if err := writeStatus(cmd, outputStatusOK, "Configuration validation passed."); err != nil {
					return err
				}
				return writeValidationWarnings(cmd, domain.CollectInstanceWarnings(set.Global, instance))
			}
			if err := writeStatus(cmd, outputStatusOK, "Configuration validation passed."); err != nil {
				return err
			}
			return writeValidationWarnings(cmd, domain.CollectConfigWarnings(set.Global, set.Instances))
		},
	}
	cmd.Flags().Bool("skip-system-ports", false, "skip system port checks")
	return cmd
}

// writeValidationWarnings 输出不会阻断配置加载的校验风险提示。
func writeValidationWarnings(cmd *cobra.Command, warnings []domain.ValidationIssue) error {
	for _, warning := range warnings {
		fields := []outputField{}
		if warning.Path != "" {
			fields = append(fields, outputKV("Path", warning.Path))
		}
		if err := writeStatus(cmd, outputStatusWarn, warning.Message, fields...); err != nil {
			return err
		}
	}
	return nil
}

// newSboxctlCheckCommand 创建 T03 已实现的 check 命令。
func newSboxctlCheckCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check [TARGET]",
		Short: "Check generation plan and service changes",
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
	cmd.Flags().Bool("skip-system-ports", false, "skip system port checks")
	return cmd
}

// newSboxctlRenderCommand 创建渲染命令组。
func newSboxctlRenderCommand() *cobra.Command {
	render := &cobra.Command{
		Use:   "render",
		Short: "Render model, sing-box configuration, or subscription input",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	render.PersistentFlags().Bool("skip-system-ports", false, "skip system port checks")
	render.AddCommand(
		newSboxctlRenderModelCommand(),
		newSboxctlRenderSingBoxCommand(),
		newSboxctlRenderSubCommand(),
	)
	return render
}

// newSboxctlRenderModelCommand 创建 render model 命令。
func newSboxctlRenderModelCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "model",
		Short: "Render the full configuration model",
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
		Short: "Render sing-box configuration for an instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			instance, ok := set.FindInstance(args[0])
			if !ok {
				return fmt.Errorf("instance %q does not exist", args[0])
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
		Short: "Render subscription input",
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
		Short:     "Export subscription configuration for a user",
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
				return fmt.Errorf("unsupported lifecycle action %q", action)
			}
			if err != nil {
				return err
			}
			if err := writePlanPreview(cmd, plan); err != nil {
				return err
			}
			if result != nil && result.Changed {
				return writeStatus(cmd, outputStatusOK, "Runtime updated and services "+action+"ed.")
			}
			return writeStatus(cmd, outputStatusOK, "Runtime already up to date; services "+action+"ed.")
		},
	}
}

// lifecycleShort 返回生命周期命令说明。
func lifecycleShort(action string) string {
	if action == "restart" {
		return "Regenerate runtime and restart instance services"
	}
	return "Generate runtime and start instance services"
}

// loadAgentSetFromCommand 根据 CLI 全局参数加载 agent 配置集合。
func loadAgentSetFromCommand(cmd *cobra.Command) (*config.AgentConfigSet, error) {
	options, err := getRootOptions(cmd)
	if err != nil {
		return nil, err
	}
	set, err := config.LoadAgentConfigSet(options.baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("agent environment is not initialized at %s; run: sboxctl --base-dir %s setup local", options.baseDir, options.baseDir)
		}
		return nil, err
	}
	return set, nil
}

// writePlanPreview 输出 runtime plan diff 预览。
func writePlanPreview(cmd *cobra.Command, plan *runtimeplan.Plan) error {
	if plan == nil || len(plan.Changes) == 0 {
		return writeStatus(cmd, outputStatusOK, "No runtime changes are required.")
	}
	rows := make([][]string, 0, len(plan.Changes))
	for _, change := range plan.Changes {
		rows = append(rows, []string{string(change.Action), change.Instance, change.RelativePath, change.Service})
	}
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "Plan"); err != nil {
		return err
	}
	return writeTable(cmd, []string{"ACTION", "INSTANCE", "FILE", "SERVICE"}, rows)
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
