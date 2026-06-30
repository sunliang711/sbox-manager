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
		Short: "Export and validate agent subscription inputs",
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
		Short: "Export a subscription bundle",
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
			return writeStatus(cmd, outputStatusOK, "Subscription bundle exported.", outputKV("File", output))
		},
	}
}

// newSboxctlSubValidateInputsCommand 创建订阅 input 目录校验命令。
func newSboxctlSubValidateInputsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-inputs",
		Short: "Validate subscription inputs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			inputDir, _ := cmd.Flags().GetString("input-dir")
			if inputDir == "" {
				return fmt.Errorf("--input-dir cannot be empty")
			}
			index, err := subscription.LoadIndexFromDir(inputDir)
			if err != nil {
				return err
			}
			return writeStatus(cmd, outputStatusOK, "Subscription inputs validation passed.",
				outputKV("Sources", fmt.Sprintf("%d", len(index.Sources))),
				outputKV("Users", fmt.Sprintf("%d", index.UserCount())),
				outputKV("Nodes", fmt.Sprintf("%d", len(index.Nodes))),
			)
		},
	}
}

// writeSubExportSummary 输出不含敏感信息的订阅导出摘要。
func writeSubExportSummary(cmd *cobra.Command, result *subscription.ExportResult, dryRun bool) error {
	if dryRun {
		if err := writeStatus(cmd, outputStatusInfo, "Dry run only; no files will be written."); err != nil {
			return err
		}
	}
	totalNodes := 0
	for _, summary := range result.Summaries {
		totalNodes += summary.Nodes
	}
	if err := writeSectionFields(cmd, "Subscription bundle",
		outputKV("Inputs", fmt.Sprintf("%d", len(result.Summaries))),
		outputKV("Nodes", fmt.Sprintf("%d", totalNodes)),
		outputKV("Template", result.Manifest.TemplateVersion),
	); err != nil {
		return err
	}
	rows := make([][]string, 0, len(result.Summaries))
	for _, summary := range result.Summaries {
		rows = append(rows, []string{summary.File, summary.Source, fmt.Sprintf("%d", summary.Nodes), fmt.Sprintf("%v", summary.Users)})
	}
	return writeTable(cmd, []string{"FILE", "SOURCE", "NODES", "USERS"}, rows)
}
