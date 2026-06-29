//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package install

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// chownTreeLike 将 path 目录树 owner 同步为 ownerPath 的 owner。
func chownTreeLike(path string, ownerPath string) error {
	targetUID, targetGID, err := pathOwner(ownerPath)
	if err != nil {
		return err
	}
	return filepath.WalkDir(path, func(item string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		currentUID, currentGID, err := pathOwner(item)
		if err != nil {
			return err
		}
		if currentUID == targetUID && currentGID == targetGID {
			return nil
		}
		if err := os.Lchown(item, targetUID, targetGID); err != nil {
			return fmt.Errorf("设置 owner %s: %w", item, err)
		}
		return nil
	})
}

// pathOwner 读取路径的 uid/gid，使用 Lstat 避免跟随 symlink。
func pathOwner(path string) (int, int, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, 0, fmt.Errorf("读取 owner %s: %w", path, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("读取 owner %s: 不支持的 stat 类型", path)
	}
	return int(stat.Uid), int(stat.Gid), nil
}
