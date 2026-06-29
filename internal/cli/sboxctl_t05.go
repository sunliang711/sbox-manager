package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/generator/singbox"
	"github.com/sunliang711/sbox-manager/internal/subscription"
)

// newSboxctlSubCommandT05 创建 T05 已实现的订阅导出命令组。
func newSboxctlSubCommandT05() *cobra.Command {
	sub := &cobra.Command{
		Use:   "sub",
		Short: "导出和校验 agent 订阅输入",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	sub.AddCommand(
		newSboxctlSubExportCommand(),
		newSboxctlSubValidateInputsCommand(),
	)
	return sub
}

// newSboxctlSubExportCommand 创建订阅 bundle 导出命令。
func newSboxctlSubExportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "export [INSTANCE]",
		Short: "导出订阅 bundle",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			set, err := loadAgentSetFromCommand(cmd)
			if err != nil {
				return err
			}
			target := optionalArg(args)
			instances, err := set.TargetInstances(target)
			if err != nil {
				return err
			}
			generatedAt := cliNow()
			inputs, err := singbox.BuildSubscriptionInputs(set.Global, instances, generatedAt)
			if err != nil {
				return err
			}
			result, err := subscription.BuildBundle(inputs, generatedAt)
			if err != nil {
				return err
			}

			summary, _ := cmd.Flags().GetBool("summary")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			if summary || dryRun {
				return writeSubExportSummary(cmd, result, dryRun)
			}

			output, _ := cmd.Flags().GetString("output")
			if output == "" {
				output = filepath.Join(set.Global.Paths.Publish, "subscription-bundle-"+generatedAt.Format("20060102-150405")+".zip")
			}
			if err := subscription.WriteFileAtomic(output, result.Data, 0640); err != nil {
				return err
			}
			if err := writeSubExportSummary(cmd, result, false); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 bundle 已导出: %s\n", output)
			return err
		},
	}
}

// newSboxctlSubValidateInputsCommand 创建订阅 input 目录校验命令。
func newSboxctlSubValidateInputsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-inputs",
		Short: "校验订阅 inputs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir, _ := cmd.Flags().GetString("input-dir")
			if inputDir == "" {
				return fmt.Errorf("--input-dir 不能为空")
			}
			index, err := subscription.LoadIndexFromDir(inputDir)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "订阅 input 校验通过: sources=%d users=%d nodes=%d\n", len(index.Sources), index.UserCount(), len(index.Nodes))
			return err
		},
	}
}

// writeSubExportSummary 输出不含敏感信息的订阅导出摘要。
func writeSubExportSummary(cmd *cobra.Command, result *subscription.ExportResult, dryRun bool) error {
	if dryRun {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), "预览模式: 不会写入文件"); err != nil {
			return err
		}
	}
	totalNodes := 0
	for _, summary := range result.Summaries {
		totalNodes += summary.Nodes
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "订阅 bundle 摘要: inputs=%d nodes=%d template=%s\n", len(result.Summaries), totalNodes, result.Manifest.TemplateVersion); err != nil {
		return err
	}
	for _, summary := range result.Summaries {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "- 输入文件 %s: source=%s nodes=%d users=%v\n", summary.File, summary.Source, summary.Nodes, summary.Users); err != nil {
			return err
		}
	}
	return nil
}
