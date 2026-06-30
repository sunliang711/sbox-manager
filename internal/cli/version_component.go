package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
)

const rulesManagedMarkerName = ".sbox-manager-managed"

// runComponentVersion 输出 sboxctl 管理的组件版本或受管资源摘要。
func runComponentVersion(cmd *cobra.Command, component string) error {
	options, err := getRootOptions(cmd)
	if err != nil {
		return err
	}
	global, err := config.LoadGlobalConfig(filepath.Join(options.baseDir, "config.yaml"), options.baseDir)
	if err != nil {
		return err
	}
	switch component {
	case "sing-box":
		return writeSingBoxVersion(cmd, *global)
	case "rules":
		return writeRulesVersion(cmd, *global)
	default:
		return fmt.Errorf("unsupported version component %q; supported values: sing-box, rules", component)
	}
}

// writeSingBoxVersion 调用受管 sing-box 二进制输出其自身版本信息。
func writeSingBoxVersion(cmd *cobra.Command, global domain.GlobalConfig) error {
	binary := filepath.Join(global.Paths.Bin, "sing-box")
	output, err := exec.CommandContext(cmd.Context(), binary, "version").CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("run %s version: %w: %s", binary, err, strings.TrimSpace(string(output)))
		}
		return fmt.Errorf("run %s version: %w", binary, err)
	}
	if len(output) == 0 {
		return writeStatus(cmd, outputStatusInfo, "sing-box returned no version output.", outputKV("Binary", binary))
	}
	_, err = cmd.OutOrStdout().Write(output)
	return err
}

// writeRulesVersion 输出受管 rules 目录和管理标记中的文件 hash 摘要。
func writeRulesVersion(cmd *cobra.Command, global domain.GlobalConfig) error {
	marker := filepath.Join(global.Paths.Rules, rulesManagedMarkerName)
	data, err := os.ReadFile(marker)
	if err != nil {
		return fmt.Errorf("read rules managed marker %s: %w", marker, err)
	}
	if err := writeSectionFields(cmd, "Rules", outputKV("Directory", global.Paths.Rules)); err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write(data)
	return err
}
