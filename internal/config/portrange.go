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
		return domain.PortRange{}, fmt.Errorf("port range must use start-end format")
	}

	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return domain.PortRange{}, fmt.Errorf("port range start must be an integer: %w", err)
	}
	end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return domain.PortRange{}, fmt.Errorf("port range end must be an integer: %w", err)
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
	return 0, fmt.Errorf("port range %d-%d has no available ports", portRange.Start, portRange.End)
}

// validateParsedPortRange 校验端口范围边界。
func validateParsedPortRange(portRange domain.PortRange) error {
	if portRange.Start < 1 || portRange.Start > 65535 {
		return fmt.Errorf("port range start must be in range 1-65535")
	}
	if portRange.End < 1 || portRange.End > 65535 {
		return fmt.Errorf("port range end must be in range 1-65535")
	}
	if portRange.Start > portRange.End {
		return fmt.Errorf("port range start must be less than or equal to end")
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
				return fmt.Errorf("port range contains unknown field %q", key)
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
		return fmt.Errorf("port range must be a start-end string or {start,end} object")
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
