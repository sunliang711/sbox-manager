package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sunliang711/sbox-manager/internal/config"
	"github.com/sunliang711/sbox-manager/internal/domain"
)

const (
	// InputDirName 是 sboxsub 受管订阅 input 目录名。
	InputDirName = "inputs"
	// BuiltinTemplateVersion 是当前内置模板版本标识。
	BuiltinTemplateVersion = "builtin-v1"
)

// InputFile 表示一个已读取并解码的订阅 input 文件。
type InputFile struct {
	Name  string
	Data  []byte
	Input domain.SubscriptionInput
}

// Index 表示按 user 聚合后的订阅索引。
type Index struct {
	BuiltAt time.Time
	Sources []string
	Nodes   []domain.SubscriptionNode
	Users   map[string][]domain.SubscriptionNode
}

// InputsDir 返回指定 base dir 下的订阅 input 目录。
func InputsDir(baseDir string) string {
	return filepath.Join(baseDir, InputDirName)
}

// LoadIndexFromDir 从目录加载全部 input 并构建索引。
func LoadIndexFromDir(dir string) (*Index, error) {
	files, err := LoadInputsFromDir(dir)
	if err != nil {
		return nil, err
	}
	return BuildIndex(files)
}

// LoadInputsFromDir 从目录按文件名稳定排序加载全部订阅 input。
func LoadInputsFromDir(dir string) ([]InputFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read subscription input directory %s: %w", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if ShouldIgnoreInputName(name) {
			continue
		}
		if !IsInputFileName(name) {
			continue
		}
		if err := domain.ValidateSubscriptionInputFilename(name); err != nil {
			return nil, fmt.Errorf("subscription input filename %s: %w", name, err)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("read subscription input file info %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	files := make([]InputFile, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read subscription input %s: %w", path, err)
		}
		input, err := DecodeInput(name, data)
		if err != nil {
			return nil, fmt.Errorf("subscription input %s: %w", path, err)
		}
		files = append(files, InputFile{Name: name, Data: data, Input: input})
	}
	return files, nil
}

// DecodeInput 按文件扩展名严格解码并校验订阅 input。
func DecodeInput(name string, data []byte) (domain.SubscriptionInput, error) {
	format, err := formatForInputName(name)
	if err != nil {
		return domain.SubscriptionInput{}, err
	}
	var input domain.SubscriptionInput
	if err := config.DecodeStrict(data, format, "SubscriptionInput", &input); err != nil {
		return domain.SubscriptionInput{}, err
	}
	if err := domain.ValidateSubscriptionInput(input); err != nil {
		return domain.SubscriptionInput{}, err
	}
	return input, nil
}

// BuildIndex 基于一组 input 文件构建可服务的订阅索引。
func BuildIndex(files []InputFile) (*Index, error) {
	inputs := make([]domain.SubscriptionInput, 0, len(files))
	for _, file := range files {
		inputs = append(inputs, file.Input)
	}
	return BuildIndexFromInputs(inputs)
}

// BuildIndexFromInputs 基于一组 input 构建可服务的订阅索引。
func BuildIndexFromInputs(inputs []domain.SubscriptionInput) (*Index, error) {
	if err := domain.ValidateSubscriptionInputs(inputs); err != nil {
		return nil, err
	}

	index := &Index{
		BuiltAt: time.Now(),
		Sources: make([]string, 0, len(inputs)),
		Users:   make(map[string][]domain.SubscriptionNode),
	}
	for _, input := range inputs {
		index.Sources = append(index.Sources, input.Source)
		for _, node := range input.Nodes {
			normalized := node
			if normalized.Server == "" {
				normalized.Server = input.ExternalHost
			}
			index.Nodes = append(index.Nodes, normalized)
			index.Users[normalized.User] = append(index.Users[normalized.User], normalized)
		}
	}
	return index, nil
}

// NodesForUser 返回指定 user 的节点副本。
func (i *Index) NodesForUser(user string) []domain.SubscriptionNode {
	if i == nil || i.Users == nil {
		return nil
	}
	nodes := i.Users[user]
	copied := make([]domain.SubscriptionNode, len(nodes))
	copy(copied, nodes)
	return copied
}

// UserCount 返回索引中包含节点的用户数量。
func (i *Index) UserCount() int {
	if i == nil {
		return 0
	}
	return len(i.Users)
}

// IsInputFileName 判断文件名是否是支持的订阅 input 扩展名。
func IsInputFileName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml", ".json":
		return true
	default:
		return false
	}
}

// ShouldIgnoreInputName 判断 watcher 和目录加载应忽略的临时或草稿文件。
func ShouldIgnoreInputName(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") {
		return true
	}
	lower := strings.ToLower(name)
	return strings.Contains(lower, ".tmp") || strings.Contains(lower, ".swp") || strings.Contains(lower, ".draft")
}

// RedactInput 返回默认展示用的脱敏订阅 input。
func RedactInput(input domain.SubscriptionInput) domain.SubscriptionInput {
	redacted := input
	redacted.Nodes = make([]domain.SubscriptionNode, len(input.Nodes))
	for index, node := range input.Nodes {
		node.UUID = redactSecret(node.UUID)
		node.Password = redactSecret(node.Password)
		node.Auth.Password = redactSecret(node.Auth.Password)
		redacted.Nodes[index] = node
	}
	return redacted
}

// MarshalStable 使用稳定 JSON 格式输出命令结果或 bundle 成员。
func MarshalStable(value interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// formatForInputName 将订阅 input 文件扩展名转换为解码格式。
func formatForInputName(name string) (string, error) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml":
		return "yaml", nil
	case ".yml":
		return "yml", nil
	case ".json":
		return "json", nil
	default:
		return "", fmt.Errorf("unsupported subscription input extension")
	}
}

// redactSecret 对非空敏感字段做固定脱敏。
func redactSecret(value string) string {
	if value == "" {
		return ""
	}
	return "[REDACTED]"
}
