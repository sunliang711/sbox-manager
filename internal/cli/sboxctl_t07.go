package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/backup"
	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/diagnostics"
)

// newSboxctlExportCommandT07 创建 agent 配置备份导出命令。
func newSboxctlExportCommandT07() *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "导出 agent 配置备份",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			generatedAt := cliNow()
			result, err := backup.Build(options.baseDir, generatedAt)
			if err != nil {
				return err
			}
			output, _ := cmd.Flags().GetString("output")
			if output == "" {
				set, err := config.LoadAgentConfigSet(options.baseDir)
				if err != nil {
					return err
				}
				output = filepath.Join(set.Global.Paths.Publish, "agent-backup-"+generatedAt.Format("20060102-150405")+".zip")
			}
			if err := backup.WriteFileAtomic(output, result.Data, 0640); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent backup 已导出: %s\n", output); err != nil {
				return err
			}
			for _, file := range result.Files {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "backup file: %s\n", file); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// newSboxctlImportCommandT07 创建 agent 配置备份导入命令。
func newSboxctlImportCommandT07() *cobra.Command {
	return &cobra.Command{
		Use:   "import BACKUP",
		Short: "导入 agent 配置备份",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			force, _ := cmd.Flags().GetBool("force")
			result, err := backup.Import(options.baseDir, args[0], force)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "agent backup 已导入: files=%d force=%t\n", len(result.Files), result.Force); err != nil {
				return err
			}
			for _, file := range result.Files {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "imported file: %s\n", file); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// newSboxctlDoctorCommandT07 创建 agent 诊断命令。
func newSboxctlDoctorCommandT07() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "执行 agent 诊断检查",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			options, err := getRootOptions(cmd)
			if err != nil {
				return err
			}
			checks := diagnostics.AgentDoctor(cmd.Context(), options.baseDir, options.serviceManager)
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

// newSboxctlIPInfoCommandT07 创建出口 IP 查询命令。
func newSboxctlIPInfoCommandT07() *cobra.Command {
	return &cobra.Command{
		Use:   "ipinfo INSTANCE",
		Short: "查询实例出口 IP 信息",
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
			family, _ := cmd.Flags().GetString("family")
			timeoutSeconds, _ := cmd.Flags().GetInt("timeout")
			timeout := time.Duration(timeoutSeconds) * time.Second
			results, err := diagnostics.LookupIPInfo(cmd.Context(), instance, diagnostics.IPInfoOptions{
				Family:  family,
				Timeout: timeout,
			})
			if err != nil {
				return err
			}
			for _, result := range results {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", result.Family, result.IP, result.Proxy.String(), result.Endpoint); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// writeDiagnosticChecks 输出 doctor 检查项。
func writeDiagnosticChecks(cmd *cobra.Command, checks []diagnostics.Check) error {
	for _, check := range checks {
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", check.Module, check.Status, check.Message); err != nil {
			return err
		}
	}
	return nil
}
