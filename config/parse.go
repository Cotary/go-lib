package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

// Parse 解析配置文件
// configPath: 配置文件路径
// fileType: 可选，配置文件类型（yaml/json/toml/hcl/ini/env/properties等）
//
//	如果为空，则尝试用文件后缀推断；如果还不行，则默认用 yaml
const (
	// YAMLConfigType YAML 配置文件类型
	YAMLConfigType = "yaml"
	// JSONConfigType JSON 配置文件类型
	JSONConfigType = "json"
	// TOMLConfigType TOML 配置文件类型
	TOMLConfigType = "toml"
	// HCLConfigType HCL 配置文件类型
	HCLConfigType = "hcl"
	// INIConfigType INI 配置文件类型
	INIConfigType = "ini"
	// ENVConfigType ENV 配置文件类型
	ENVConfigType = "env"
	// PropertiesConfigType Properties 配置文件类型
	PropertiesConfigType = "properties"
)

func Parse(configPath string, fileType string, conf any) error {
	v := viper.New()
	v.SetConfigFile(configPath)

	// 1. 如果调用方传了 fileType，直接使用
	if fileType != "" {
		v.SetConfigType(strings.ToLower(fileType))
	} else {
		// 2. 没传则尝试用文件后缀推断
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(configPath)), ".")
		if ext != "" {
			v.SetConfigType(ext)
		} else {
			// 3. 没有后缀则默认用 yaml
			v.SetConfigType(YAMLConfigType)
		}
	}

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return errors.Wrap(err, fmt.Sprintf("config file read err: %s", configPath))
	}

	// 将配置映射到结构体（需使用 mapstructure 标签）
	if err := v.Unmarshal(conf); err != nil {
		return errors.Wrap(err, "config file unmarshal err")
	}

	return nil
}
