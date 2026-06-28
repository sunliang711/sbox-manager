package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sunliang711/sbox-manager/internal/domain"
)

// ParsePortRange 解析 start-end 格式端口范围。
func ParsePortRange(text string) (domain.PortRange, error) {
	parts := strings.Split(strings.TrimSpace(text), "-")
	if len(parts) != 2 {
		return domain.PortRange{}, fmt.Errorf("端口范围必须是 start-end 格式")
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return domain.PortRange{}, fmt.Errorf("端口范围 start 必须是整数: %w", err)
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return domain.PortRange{}, fmt.Errorf("端口范围 end 必须是整数: %w", err)
	}
	portRange := domain.PortRange{Start: start, End: end}
	if err := validateParsedPortRange(portRange); err != nil {
		return domain.PortRange{}, err
	}
	return portRange, nil
}

// FirstAvailablePort 返回范围内第一个未占用端口。
func FirstAvailablePort(portRange domain.PortRange, used map[int]struct{}) (int, error) {
	if err := validateParsedPortRange(portRange); err != nil {
		return 0, err
	}
	for port := portRange.Start; port <= portRange.End; port++ {
		if _, exists := used[port]; !exists {
			return port, nil
		}
	}
	return 0, fmt.Errorf("端口范围 %d-%d 已无可用端口", portRange.Start, portRange.End)
}

// validateParsedPortRange 校验端口范围边界。
func validateParsedPortRange(portRange domain.PortRange) error {
	if portRange.Start < 1 || portRange.Start > 65535 {
		return fmt.Errorf("端口范围 start 必须在 1-65535 范围内")
	}
	if portRange.End < 1 || portRange.End > 65535 {
		return fmt.Errorf("端口范围 end 必须在 1-65535 范围内")
	}
	if portRange.Start > portRange.End {
		return fmt.Errorf("端口范围 start 必须小于或等于 end")
	}
	return nil
}

type portRangeValue struct {
	value domain.PortRange
}

// UnmarshalYAML 支持 YAML 中的 start-end 字符串和 {start,end} 对象。
func (v *portRangeValue) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		portRange, err := ParsePortRange(node.Value)
		if err != nil {
			return err
		}
		v.value = portRange
		return nil
	case yaml.MappingNode:
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index].Value
			if key != "start" && key != "end" {
				return fmt.Errorf("端口范围包含未知字段 %q", key)
			}
		}
		var decoded struct {
			Start int `yaml:"start"`
			End   int `yaml:"end"`
		}
		if err := node.Decode(&decoded); err != nil {
			return err
		}
		portRange := domain.PortRange{Start: decoded.Start, End: decoded.End}
		if err := validateParsedPortRange(portRange); err != nil {
			return err
		}
		v.value = portRange
		return nil
	default:
		return fmt.Errorf("端口范围必须是 start-end 字符串或 {start,end} 对象")
	}
}

// UnmarshalJSON 支持 JSON 中的 start-end 字符串和 {"start","end"} 对象。
func (v *portRangeValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		portRange, parseErr := ParsePortRange(text)
		if parseErr != nil {
			return parseErr
		}
		v.value = portRange
		return nil
	}

	var decoded struct {
		Start int `json:"start"`
		End   int `json:"end"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	portRange := domain.PortRange{Start: decoded.Start, End: decoded.End}
	if err := validateParsedPortRange(portRange); err != nil {
		return err
	}
	v.value = portRange
	return nil
}
