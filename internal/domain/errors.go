package domain

import (
	"fmt"
	"strings"
)

// ValidationIssue 表示一条配置校验问题。
type ValidationIssue struct {
	Path    string
	Message string
}

// ValidationErrors 聚合多条配置校验问题。
type ValidationErrors struct {
	Issues []ValidationIssue
}

// Error 返回所有校验问题的单行摘要。
func (e *ValidationErrors) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "配置校验失败"
	}

	parts := make([]string, 0, len(e.Issues))
	for _, issue := range e.Issues {
		if issue.Path == "" {
			parts = append(parts, issue.Message)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", issue.Path, issue.Message))
	}
	return "配置校验失败: " + strings.Join(parts, "; ")
}

// Add 追加一条校验问题。
func (e *ValidationErrors) Add(path string, format string, args ...interface{}) {
	e.Issues = append(e.Issues, ValidationIssue{
		Path:    path,
		Message: fmt.Sprintf(format, args...),
	})
}

// HasIssues 返回是否已经收集到校验问题。
func (e *ValidationErrors) HasIssues() bool {
	return e != nil && len(e.Issues) > 0
}

// ErrOrNil 在存在问题时返回聚合错误，否则返回 nil。
func (e *ValidationErrors) ErrOrNil() error {
	if e.HasIssues() {
		return e
	}
	return nil
}
