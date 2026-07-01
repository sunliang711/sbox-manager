package templates

import "embed"

// SubFS 保存订阅默认 Jinja 模板，供 sboxsub 作为内置模板读取。
//
//go:embed sub/*.j2
var SubFS embed.FS
