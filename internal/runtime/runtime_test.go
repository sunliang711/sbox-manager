package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// TestBuildPlanIsReadOnly 验证 check/plan 构建不写 generated 或 manifest。
func TestBuildPlanIsReadOnly(t *testing.T) {
	global, instances := runtimeFixture(t, "edge-b")

	plan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Changes) != 1 || plan.Changes[0].Action != ActionCreate {
		t.Fatalf("expected create plan, got %+v", plan.Changes)
	}
	if _, err := os.Stat(global.Paths.Generated); !os.IsNotExist(err) {
		t.Fatalf("generated dir should not be written, stat err: %v", err)
	}
	if _, err := os.Stat(plan.ManifestPath); !os.IsNotExist(err) {
		t.Fatalf("manifest should not be written, stat err: %v", err)
	}
}

// TestApplyPlanWritesManifestSorted 验证 apply 后 manifest 内容和 files 排序。
func TestApplyPlanWritesManifestSorted(t *testing.T) {
	global, instances := runtimeFixture(t, "edge-b", "edge-a")

	plan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	result, err := ApplyPlan(plan, fixedClock)
	if err != nil {
		t.Fatalf("apply plan: %v", err)
	}
	if result == nil || !result.Changed {
		t.Fatal("expected changed apply result")
	}

	manifest, err := LoadManifest(plan.ManifestPath, global.Paths.Generated)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if manifest.GeneratedAt != fixedClock() {
		t.Fatalf("unexpected generated_at: %s", manifest.GeneratedAt)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(manifest.Files))
	}
	if manifest.Files[0].RelativePath > manifest.Files[1].RelativePath {
		t.Fatalf("manifest files are not sorted: %+v", manifest.Files)
	}
	if mode := fileMode(t, plan.ManifestPath); mode != 0640 {
		t.Fatalf("expected manifest mode 0640, got %o", mode)
	}
	for _, file := range manifest.Files {
		if mode := fileMode(t, file.Path); mode != 0640 {
			t.Fatalf("expected generated mode 0640, got %o", mode)
		}
	}
}

// TestApplyPlanNoChangePreservesMTime 验证 no-change 不改写 generated 或 manifest。
func TestApplyPlanNoChangePreservesMTime(t *testing.T) {
	global, instances := runtimeFixture(t, "edge-us")
	firstPlan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build first plan: %v", err)
	}
	if _, err := ApplyPlan(firstPlan, fixedClock); err != nil {
		t.Fatalf("apply first plan: %v", err)
	}

	generatedPath := firstPlan.DesiredFiles[0].Path
	manifestPath := firstPlan.ManifestPath
	generatedMTime := modTime(t, generatedPath)
	manifestMTime := modTime(t, manifestPath)
	time.Sleep(20 * time.Millisecond)

	secondPlan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build second plan: %v", err)
	}
	for _, change := range secondPlan.Changes {
		if change.Action != ActionNoChange {
			t.Fatalf("expected no-change, got %+v", secondPlan.Changes)
		}
	}
	result, err := ApplyPlan(secondPlan, func() string {
		return "2099-01-01T00:00:00+08:00"
	})
	if err != nil {
		t.Fatalf("apply no-change plan: %v", err)
	}
	if result.Changed {
		t.Fatal("expected no-change apply result")
	}
	if got := modTime(t, generatedPath); !got.Equal(generatedMTime) {
		t.Fatalf("generated mtime changed: %s -> %s", generatedMTime, got)
	}
	if got := modTime(t, manifestPath); !got.Equal(manifestMTime) {
		t.Fatalf("manifest mtime changed: %s -> %s", manifestMTime, got)
	}
}

// TestCommandConfigCheckerMissingBinaryReturnsError 验证缺少 sing-box 时默认 checker 不能静默成功。
func TestCommandConfigCheckerMissingBinaryReturnsError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := CommandConfigChecker{}.Check(context.Background(), "edge-us", []byte(`{"log":{"level":"info"}}`))
	if err == nil {
		t.Fatal("expected missing sing-box error")
	}
	if !strings.Contains(err.Error(), "sing-box") {
		t.Fatalf("expected sing-box error, got %v", err)
	}
}

// TestStartMissingCheckerBinaryDoesNotWriteRuntime 验证 checker 失败时 start 不写 generated 或 manifest。
func TestStartMissingCheckerBinaryDoesNotWriteRuntime(t *testing.T) {
	global, instances := runtimeFixture(t, "edge-us")
	plan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	manager := &fakeServiceManager{}
	checker := CommandConfigChecker{
		Binary: filepath.Join(t.TempDir(), "missing-sing-box"),
	}
	if _, err := Start(context.Background(), plan, checker, manager, fixedClock); err == nil {
		t.Fatal("expected missing checker binary error")
	}
	if manager.startCalls != 0 {
		t.Fatalf("service manager should not be called, got %d", manager.startCalls)
	}
	if _, err := os.Stat(global.Paths.Generated); !os.IsNotExist(err) {
		t.Fatalf("generated dir should not be written, stat err: %v", err)
	}
	if _, err := os.Stat(plan.ManifestPath); !os.IsNotExist(err) {
		t.Fatalf("manifest should not be written, stat err: %v", err)
	}
}

// TestLifecycleNilCheckerDoesNotWriteRuntime 验证 nil checker 不会退回 noop 并产生副作用。
func TestLifecycleNilCheckerDoesNotWriteRuntime(t *testing.T) {
	global, instances := runtimeFixture(t, "edge-us")
	plan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}

	startManager := &fakeServiceManager{}
	if _, err := Start(context.Background(), plan, nil, startManager, fixedClock); err == nil {
		t.Fatal("expected nil checker start error")
	}
	if startManager.startCalls != 0 {
		t.Fatalf("service manager should not be called, got %d", startManager.startCalls)
	}
	assertRuntimeNotWritten(t, global.Paths.Generated, plan.ManifestPath)

	restartManager := &fakeServiceManager{}
	if _, err := Restart(context.Background(), plan, nil, restartManager, fixedClock); err == nil {
		t.Fatal("expected nil checker restart error")
	}
	if restartManager.restartCalls != 0 {
		t.Fatalf("service manager should not be called, got %d", restartManager.restartCalls)
	}
	assertRuntimeNotWritten(t, global.Paths.Generated, plan.ManifestPath)
}

// TestLifecycleNoChangeStillCallsService 验证 start/restart no-change 仍调用服务管理器。
func TestLifecycleNoChangeStillCallsService(t *testing.T) {
	global, instances := runtimeFixture(t, "edge-us")
	firstPlan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build first plan: %v", err)
	}
	if _, err := ApplyPlan(firstPlan, fixedClock); err != nil {
		t.Fatalf("apply first plan: %v", err)
	}

	startPlan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build start plan: %v", err)
	}
	startManager := &fakeServiceManager{}
	startChecker := &fakeChecker{}
	result, err := Start(context.Background(), startPlan, startChecker, startManager, fixedClock)
	if err != nil {
		t.Fatalf("start no-change plan: %v", err)
	}
	if result.Changed {
		t.Fatal("start no-change should not rewrite runtime")
	}
	if startManager.startCalls != 1 {
		t.Fatalf("expected one start call, got %d", startManager.startCalls)
	}
	if startChecker.calls != 1 {
		t.Fatalf("expected one checker call, got %d", startChecker.calls)
	}

	restartPlan, err := BuildPlan(global, instances, "")
	if err != nil {
		t.Fatalf("build restart plan: %v", err)
	}
	restartManager := &fakeServiceManager{}
	if _, err := Restart(context.Background(), restartPlan, NoopConfigChecker{}, restartManager, fixedClock); err != nil {
		t.Fatalf("restart no-change plan: %v", err)
	}
	if restartManager.restartCalls != 1 {
		t.Fatalf("expected one restart call, got %d", restartManager.restartCalls)
	}
}

// assertRuntimeNotWritten 验证 generated 和 manifest 都未写入。
func assertRuntimeNotWritten(t *testing.T, generatedDir string, manifestPath string) {
	t.Helper()
	if _, err := os.Stat(generatedDir); !os.IsNotExist(err) {
		t.Fatalf("generated dir should not be written, stat err: %v", err)
	}
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("manifest should not be written, stat err: %v", err)
	}
}

// runtimeFixture 返回 runtime 测试用配置。
func runtimeFixture(t *testing.T, names ...string) (domain.GlobalConfig, []domain.Instance) {
	t.Helper()
	baseDir := t.TempDir()
	global := domain.DefaultGlobalConfig()
	global.ExternalHost = "proxy.example.com"
	global.Paths.Runtime = filepath.Join(baseDir, "runtime")
	global.Paths.Generated = filepath.Join(baseDir, "runtime", "generated")
	global.Paths.Instances = filepath.Join(baseDir, "instances")

	instances := make([]domain.Instance, 0, len(names))
	for index, name := range names {
		instance := domain.DefaultInstance(global)
		instance.Name = name
		instance.API.Enabled = false
		instance.Inbounds = []domain.Inbound{
			{
				Name:   "vmess-main",
				Type:   "vmess",
				Listen: "0.0.0.0",
				Port:   24100 + index,
				Users: []domain.InboundUser{
					{
						Name: "alice",
						UUID: "11111111-1111-4111-8111-111111111111",
					},
				},
			},
		}
		instance.Outbounds = []domain.Outbound{
			{Name: "direct", Type: "direct"},
		}
		instance.Route = domain.RouteConfig{Default: "direct"}
		domain.ApplyInstanceDefaults(&instance)
		instances = append(instances, instance)
	}
	return global, instances
}

// fixedClock 返回测试固定 generated_at。
func fixedClock() string {
	return "2026-06-28T12:00:00+08:00"
}

// fileMode 返回文件权限位。
func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// modTime 返回文件修改时间。
func modTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file %s: %v", path, err)
	}
	return info.ModTime()
}

type fakeChecker struct {
	calls int
}

// Check 记录 checker 调用次数。
func (f *fakeChecker) Check(ctx context.Context, instance string, data []byte) error {
	f.calls++
	return nil
}

type fakeServiceManager struct {
	startCalls   int
	restartCalls int
}

// Start 记录 start 调用次数。
func (f *fakeServiceManager) Start(ctx context.Context, services []string) error {
	f.startCalls++
	return nil
}

// Restart 记录 restart 调用次数。
func (f *fakeServiceManager) Restart(ctx context.Context, services []string) error {
	f.restartCalls++
	return nil
}
