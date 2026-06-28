package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// DecodeStrict 按格式严格解码配置数据，未知字段会返回错误。
func DecodeStrict(data []byte, format string, schema string, target interface{}) error {
	if strings.TrimSpace(schema) == "" {
		schema = "unknown schema"
	}
	if err := decodeStrictReader(bytes.NewReader(data), format, target); err != nil {
		return fmt.Errorf("%s: %w", schema, err)
	}
	return nil
}

// LoadGlobalConfig 严格加载并校验 agent 全局配置。
func LoadGlobalConfig(path string, baseDir string) (*domain.GlobalConfig, error) {
	defaultConfig := domain.DefaultGlobalConfig()
	raw := newGlobalConfigFile(defaultConfig)
	if err := loadStrictFile(path, "GlobalConfig", &raw); err != nil {
		return nil, err
	}

	config := raw.toDomain()
	if err := NormalizeGlobalPaths(baseDir, &config); err != nil {
		return nil, fmt.Errorf("GlobalConfig %s: %w", path, err)
	}
	if err := domain.ValidateGlobalConfig(config); err != nil {
		return nil, fmt.Errorf("GlobalConfig %s: %w", path, err)
	}
	return &config, nil
}

// LoadInstance 严格加载并校验 instance 配置。
func LoadInstance(path string, global domain.GlobalConfig) (*domain.Instance, error) {
	instance := domain.DefaultInstance(global)
	if err := loadStrictFile(path, "Instance", &instance); err != nil {
		return nil, err
	}
	domain.ApplyInstanceDefaults(&instance)
	if err := validateInstanceFilename(path, instance.Name); err != nil {
		return nil, fmt.Errorf("Instance %s: %w", path, err)
	}
	if err := domain.ValidateInstance(global, &instance); err != nil {
		return nil, fmt.Errorf("Instance %s: %w", path, err)
	}
	return &instance, nil
}

// LoadSubConfig 严格加载并校验订阅服务配置。
func LoadSubConfig(path string, baseDir string) (*domain.SubConfig, error) {
	raw := newSubConfigFile(domain.DefaultSubConfig())
	if err := loadStrictFile(path, "SubConfig", &raw); err != nil {
		return nil, err
	}

	config := raw.toDomain()
	if err := NormalizeSubConfigPaths(baseDir, &config); err != nil {
		return nil, fmt.Errorf("SubConfig %s: %w", path, err)
	}
	if err := domain.ValidateSubConfig(config); err != nil {
		return nil, fmt.Errorf("SubConfig %s: %w", path, err)
	}
	return &config, nil
}

// LoadTrafficConfig 严格加载并校验独立 traffic 配置。
func LoadTrafficConfig(path string) (*domain.TrafficConfig, error) {
	config := domain.DefaultTrafficConfig()
	if err := loadStrictFile(path, "TrafficConfig", &config); err != nil {
		return nil, err
	}
	if err := domain.ValidateTrafficConfig(config); err != nil {
		return nil, fmt.Errorf("TrafficConfig %s: %w", path, err)
	}
	return &config, nil
}

// LoadSubscriptionInput 严格加载订阅 input。
func LoadSubscriptionInput(path string) (*domain.SubscriptionInput, error) {
	var input domain.SubscriptionInput
	if err := loadStrictFile(path, "SubscriptionInput", &input); err != nil {
		return nil, err
	}
	if err := domain.ValidateSubscriptionInput(input); err != nil {
		return nil, fmt.Errorf("SubscriptionInput %s: %w", path, err)
	}
	return &input, nil
}

// LoadSubscriptionIndex 严格加载订阅 index。
func LoadSubscriptionIndex(path string) (*domain.SubscriptionIndex, error) {
	var index domain.SubscriptionIndex
	if err := loadStrictFile(path, "SubscriptionIndex", &index); err != nil {
		return nil, err
	}
	return &index, nil
}

// LoadBundleManifest 严格加载订阅 bundle manifest。
func LoadBundleManifest(path string) (*domain.BundleManifest, error) {
	var manifest domain.BundleManifest
	if err := loadStrictFile(path, "BundleManifest", &manifest); err != nil {
		return nil, err
	}
	if err := domain.ValidateBundleManifest(manifest); err != nil {
		return nil, fmt.Errorf("BundleManifest %s: %w", path, err)
	}
	return &manifest, nil
}

// LoadBackupManifest 严格加载 agent backup manifest。
func LoadBackupManifest(path string) (*domain.BackupManifest, error) {
	var manifest domain.BackupManifest
	if err := loadStrictFile(path, "BackupManifest", &manifest); err != nil {
		return nil, err
	}
	if err := domain.ValidateBackupManifest(manifest); err != nil {
		return nil, fmt.Errorf("BackupManifest %s: %w", path, err)
	}
	return &manifest, nil
}

// loadStrictFile 读取文件并按扩展名严格解码。
func loadStrictFile(path string, schema string, target interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s %s: %w", schema, path, err)
	}
	format, err := formatForPath(path)
	if err != nil {
		return fmt.Errorf("%s %s: %w", schema, path, err)
	}
	if err := DecodeStrict(data, format, schema, target); err != nil {
		return fmt.Errorf("%s %s: %w", schema, path, err)
	}
	return nil
}

// decodeStrictReader 按格式从 reader 严格解码。
func decodeStrictReader(reader io.Reader, format string, target interface{}) error {
	switch strings.ToLower(format) {
	case "yaml", "yml":
		decoder := yaml.NewDecoder(reader)
		decoder.KnownFields(true)
		if err := decoder.Decode(target); err != nil {
			return err
		}

		// 严格模式只允许一个 YAML document，避免第二个 document 绕过未知字段校验。
		var extra interface{}
		if err := decoder.Decode(&extra); err != io.EOF {
			if err != nil {
				return err
			}
			return fmt.Errorf("YAML 只允许单文档")
		}
		return nil
	case "json":
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			return err
		}

		// 严格模式只允许一个 JSON 顶层值，避免尾随对象被静默忽略。
		var extra interface{}
		if err := decoder.Decode(&extra); err != io.EOF {
			if err != nil {
				return err
			}
			return fmt.Errorf("JSON 只允许单个顶层值")
		}
		return nil
	default:
		return fmt.Errorf("不支持的配置格式 %q", format)
	}
}

// formatForPath 根据文件扩展名返回解码格式。
func formatForPath(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return "yaml", nil
	case ".json":
		return "json", nil
	default:
		return "", fmt.Errorf("不支持的配置文件扩展名")
	}
}

// validateInstanceFilename 校验 YAML instance 文件名与 name 一致。
func validateInstanceFilename(path string, name string) error {
	extension := strings.ToLower(filepath.Ext(path))
	if extension != ".yaml" && extension != ".yml" {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(path), extension)
	if base != name {
		return fmt.Errorf("文件名 %q 必须与 instance name %q 一致", base, name)
	}
	return nil
}

type globalConfigFile struct {
	Version      int                   `yaml:"version" json:"version"`
	ExternalHost string                `yaml:"external_host" json:"external_host"`
	Paths        domain.PathsConfig    `yaml:"paths" json:"paths"`
	PortRanges   portRangesConfigFile  `yaml:"port_ranges" json:"port_ranges"`
	Defaults     domain.DefaultsConfig `yaml:"defaults" json:"defaults"`
	Security     domain.SecurityConfig `yaml:"security" json:"security"`
}

// newGlobalConfigFile 将领域默认值转换为加载专用结构。
func newGlobalConfigFile(config domain.GlobalConfig) globalConfigFile {
	return globalConfigFile{
		Version:      config.Version,
		ExternalHost: config.ExternalHost,
		Paths:        config.Paths,
		PortRanges: portRangesConfigFile{
			Inbound:    portRangeValue{value: config.PortRanges.Inbound},
			LocalSocks: portRangeValue{value: config.PortRanges.LocalSocks},
			LocalHTTP:  portRangeValue{value: config.PortRanges.LocalHTTP},
			API:        portRangeValue{value: config.PortRanges.API},
		},
		Defaults: config.Defaults,
		Security: config.Security,
	}
}

// toDomain 将加载专用全局配置转换为领域模型。
func (f globalConfigFile) toDomain() domain.GlobalConfig {
	return domain.GlobalConfig{
		Version:      f.Version,
		ExternalHost: f.ExternalHost,
		Paths:        f.Paths,
		PortRanges: domain.PortRangesConfig{
			Inbound:    f.PortRanges.Inbound.value,
			LocalSocks: f.PortRanges.LocalSocks.value,
			LocalHTTP:  f.PortRanges.LocalHTTP.value,
			API:        f.PortRanges.API.value,
		},
		Defaults: f.Defaults,
		Security: f.Security,
	}
}

type portRangesConfigFile struct {
	Inbound    portRangeValue `yaml:"inbound" json:"inbound"`
	LocalSocks portRangeValue `yaml:"local_socks" json:"local_socks"`
	LocalHTTP  portRangeValue `yaml:"local_http" json:"local_http"`
	API        portRangeValue `yaml:"api" json:"api"`
}

type subConfigFile struct {
	Version       int                  `yaml:"version" json:"version"`
	Listen        string               `yaml:"listen" json:"listen"`
	Access        domain.AccessConfig  `yaml:"access" json:"access"`
	TemplatesDir  string               `yaml:"templates_dir" json:"templates_dir"`
	WatchInterval durationValue        `yaml:"watch_interval" json:"watch_interval"`
	WatchDebounce durationValue        `yaml:"watch_debounce" json:"watch_debounce"`
	ManagedConfig domain.ManagedConfig `yaml:"managed_config" json:"managed_config"`
}

// newSubConfigFile 将领域默认值转换为订阅服务加载专用结构。
func newSubConfigFile(config domain.SubConfig) subConfigFile {
	return subConfigFile{
		Version:      config.Version,
		Listen:       config.Listen,
		Access:       config.Access,
		TemplatesDir: config.TemplatesDir,
		WatchInterval: durationValue{
			value: config.WatchInterval,
		},
		WatchDebounce: durationValue{
			value: config.WatchDebounce,
		},
		ManagedConfig: config.ManagedConfig,
	}
}

// toDomain 将加载专用订阅服务配置转换为领域模型。
func (f subConfigFile) toDomain() domain.SubConfig {
	return domain.SubConfig{
		Version:       f.Version,
		Listen:        f.Listen,
		Access:        f.Access,
		TemplatesDir:  f.TemplatesDir,
		WatchInterval: f.WatchInterval.value,
		WatchDebounce: f.WatchDebounce.value,
		ManagedConfig: f.ManagedConfig,
	}
}

type durationValue struct {
	value time.Duration
}

// UnmarshalYAML 解析 YAML duration 字符串。
func (v *durationValue) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration 必须是字符串")
	}
	duration, err := time.ParseDuration(node.Value)
	if err != nil {
		return fmt.Errorf("duration 格式无效: %w", err)
	}
	v.value = duration
	return nil
}

// UnmarshalJSON 解析 JSON duration 字符串。
func (v *durationValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	duration, err := time.ParseDuration(text)
	if err != nil {
		return fmt.Errorf("duration 格式无效: %w", err)
	}
	v.value = duration
	return nil
}
