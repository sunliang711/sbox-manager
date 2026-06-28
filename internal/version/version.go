package version

// Version 表示当前二进制版本，可由 Makefile 通过 ldflags 注入。
var Version = "dev"

// Commit 表示当前构建对应的 Git commit，可由 Makefile 通过 ldflags 注入。
var Commit = "unknown"

// BuildTime 表示当前构建时间，可由 Makefile 通过 ldflags 注入。
var BuildTime = "unknown"

// Info 保存当前二进制的版本元信息。
type Info struct {
	Version   string
	Commit    string
	BuildTime string
}

// Get 返回当前二进制的版本元信息。
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}
}
