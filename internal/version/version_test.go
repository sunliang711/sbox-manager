package version

import "testing"

// TestGet 验证 Get 返回当前全局版本字段。
func TestGet(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldBuildTime := BuildTime
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		BuildTime = oldBuildTime
	})

	Version = "v0.1.0"
	Commit = "123abcd"
	BuildTime = "2026-06-28T02:00:00Z"

	got := Get()
	if got.Version != Version {
		t.Fatalf("Version = %q, want %q", got.Version, Version)
	}
	if got.Commit != Commit {
		t.Fatalf("Commit = %q, want %q", got.Commit, Commit)
	}
	if got.BuildTime != BuildTime {
		t.Fatalf("BuildTime = %q, want %q", got.BuildTime, BuildTime)
	}
}
