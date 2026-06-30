package runtime

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/sunliang711/sbox-manager/internal/generator/singbox"
)

const managedFileMode os.FileMode = 0640
const serviceOwnerName = "sbox"

// DefaultClock 返回当前本地时间的 RFC3339 字符串。
func DefaultClock() string {
	return time.Now().Format(time.RFC3339)
}

// ApplyPlan 原子应用 runtime plan，no-change 时不改写 generated 或 manifest。
func ApplyPlan(plan *Plan, clock Clock) (*ApplyResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan cannot be empty")
	}
	if clock == nil {
		clock = DefaultClock
	}
	if !plan.HasChanges() {
		// no-change 必须直接返回，避免改写 generated 文件或 manifest 的 mtime/generated_at。
		return &ApplyResult{
			Manifest: plan.Existing,
			Changed:  false,
		}, nil
	}

	for _, change := range plan.Changes {
		switch change.Action {
		case ActionCreate, ActionUpdate:
			if err := writeFileAtomic(change.Path, change.Data, managedFileMode); err != nil {
				return nil, err
			}
		case ActionDelete:
			if err := removeManagedFile(change.Path, plan.GeneratedDir); err != nil {
				return nil, err
			}
		case ActionNoChange:
			continue
		default:
			return nil, fmt.Errorf("unknown runtime action %q", change.Action)
		}
	}

	manifest := buildAppliedManifest(plan, clock())
	if err := writeManifest(plan.ManifestPath, manifest); err != nil {
		return nil, err
	}
	return &ApplyResult{
		Manifest: &manifest,
		Changed:  true,
	}, nil
}

// buildAppliedManifest 根据 apply 后状态构造新的 manifest。
func buildAppliedManifest(plan *Plan, generatedAt string) Manifest {
	filesByRelative := map[string]ManifestFile{}
	instanceSHA256 := map[string]string{}

	if plan.Existing != nil && !plan.fullScope {
		for key, value := range plan.Existing.InstanceSHA256 {
			if _, inScope := plan.targetScope[key]; !inScope {
				instanceSHA256[key] = value
			}
		}
		for _, file := range plan.Existing.Files {
			if !plan.inTargetScope(file.Instance) {
				filesByRelative[file.RelativePath] = file
			}
		}
	}

	for _, desired := range plan.DesiredFiles {
		filesByRelative[desired.RelativePath] = desired.ManifestFile
		instanceSHA256[desired.Instance] = plan.InstanceSHA256[desired.Instance]
	}

	files := make([]ManifestFile, 0, len(filesByRelative))
	for _, file := range filesByRelative {
		files = append(files, file)
	}
	SortManifestFiles(files)
	return Manifest{
		ManifestSchema:  ManifestSchema,
		ManifestVersion: ManifestVersion,
		ConfigSHA256:    plan.ConfigSHA256,
		InstanceSHA256:  instanceSHA256,
		GeneratedAt:     generatedAt,
		Files:           files,
	}
}

// writeManifest 以 runtime manifest 权限原子写入 manifest。
func writeManifest(path string, manifest Manifest) error {
	data, err := singbox.MarshalStable(manifest)
	if err != nil {
		return fmt.Errorf("encode runtime manifest: %w", err)
	}
	return writeFileAtomic(path, data, managedFileMode)
}

// removeManagedFile 删除 generated 下的受管文件，文件不存在时视为成功。
func removeManagedFile(path string, generatedDir string) error {
	if !isPathUnder(generatedDir, path) {
		return fmt.Errorf("refuse to delete file outside generated directory: %s", path)
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("delete generated file %s: %w", path, err)
	}
	return syncDir(filepath.Dir(path))
}

// writeFileAtomic 使用同目录临时文件、fsync 和 rename 原子写入文件。
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file %s: %w", dir, err)
	}
	tempPath := tempFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tempFile.Close()
		}
		_ = os.Remove(tempPath)
	}()

	if err := tempFile.Chmod(mode); err != nil {
		return fmt.Errorf("set temp file permissions %s: %w", tempPath, err)
	}
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("write temp file %s: %w", tempPath, err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp file %s: %w", tempPath, err)
	}
	if err := tempFile.Close(); err != nil {
		closed = true
		return fmt.Errorf("close temp file %s: %w", tempPath, err)
	}
	closed = true

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace file %s: %w", path, err)
	}
	if err := applyServiceOwnership(path); err != nil {
		return err
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	return nil
}

// applyServiceOwnership 在 Linux root 场景下让 systemd sbox 用户可读取新生成的 runtime 文件。
func applyServiceOwnership(path string) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	uid, gid, ok := serviceOwnerIDs()
	if !ok {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.Chown(dir, uid, gid); err != nil {
		return fmt.Errorf("set runtime directory owner %s: %w", dir, err)
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("set runtime file owner %s: %w", path, err)
	}
	return nil
}

// serviceOwnerIDs 返回 systemd 服务用户的 uid/gid，用户不存在时表示无需处理。
func serviceOwnerIDs() (int, int, bool) {
	serviceUser, err := user.Lookup(serviceOwnerName)
	if err != nil {
		return 0, 0, false
	}
	uid, err := strconv.Atoi(serviceUser.Uid)
	if err != nil {
		return 0, 0, false
	}
	gid, err := strconv.Atoi(serviceUser.Gid)
	if err != nil {
		return 0, 0, false
	}
	return uid, gid, true
}

// syncDir fsync 目录，确保 rename 元数据尽量落盘。
func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory %s: %w", dir, err)
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return fmt.Errorf("sync directory %s: %w", dir, err)
	}
	return nil
}
