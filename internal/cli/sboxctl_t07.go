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
		Short: "Export an agent config backup",
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
			if err := writeStatus(cmd, outputStatusOK, "Agent backup exported.",
				outputKV("Archive", output),
				outputKV("Files", fmt.Sprintf("%d", len(result.Files))),
			); err != nil {
				return err
			}
			rows := make([][]string, 0, len(result.Files))
			for _, file := range result.Files {
				rows = append(rows, []string{file})
			}
			return writeTable(cmd, []string{"BACKED UP FILE"}, rows)
		},
	}
}

// newSboxctlImportCommandT07 创建 agent 配置备份导入命令。
func newSboxctlImportCommandT07() *cobra.Command {
	return &cobra.Command{
		Use:   "import BACKUP",
		Short: "Import an agent config backup",
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
			if err := writeStatus(cmd, outputStatusOK, "Agent backup imported.",
				outputKV("Files", fmt.Sprintf("%d", len(result.Files))),
				outputKV("Force", fmt.Sprintf("%t", result.Force)),
			); err != nil {
				return err
			}
			rows := make([][]string, 0, len(result.Files))
			for _, file := range result.Files {
				rows = append(rows, []string{file})
			}
			return writeTable(cmd, []string{"IMPORTED FILE"}, rows)
		},
	}
}

// newSboxctlDoctorCommandT07 创建 agent 诊断命令。
func newSboxctlDoctorCommandT07() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run agent diagnostics",
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
				return fmt.Errorf("doctor found issues")
			}
			return nil
		},
	}
}

// newSboxctlIPInfoCommandT07 创建出口 IP 查询命令。
func newSboxctlIPInfoCommandT07() *cobra.Command {
	return &cobra.Command{
		Use:   "ipinfo INSTANCE",
		Short: "Query instance egress IP information",
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
			rows := make([][]string, 0, len(results))
			for _, result := range results {
				rows = append(rows, []string{result.Family, result.IP, result.Proxy.String(), result.Endpoint})
			}
			return writeTable(cmd, []string{"FAMILY", "IP", "PROXY", "ENDPOINT"}, rows)
		},
	}
}

// writeDiagnosticChecks 输出 doctor 检查项。
func writeDiagnosticChecks(cmd *cobra.Command, checks []diagnostics.Check) error {
	okCount := 0
	issueCount := 0
	rows := make([][]string, 0, len(checks))
	for _, check := range checks {
		switch check.Status {
		case diagnostics.StatusOK:
			okCount++
		case diagnostics.StatusIssue:
			issueCount++
		}
		rows = append(rows, []string{check.Module, check.Status, check.Message})
	}
	if err := writeSectionFields(cmd, "Doctor",
		outputKV("OK", fmt.Sprintf("%d", okCount)),
		outputKV("Issues", fmt.Sprintf("%d", issueCount)),
	); err != nil {
		return err
	}
	return writeTable(cmd, []string{"CHECK", "STATUS", "MESSAGE"}, rows)
}
