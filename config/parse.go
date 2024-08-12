package config

import (
	"fmt"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
	"os"
)

func Parse(configPath string, conf any) error {
	b, err := os.ReadFile(configPath)
	if err != nil {
		return errors.New(fmt.Sprintf("config file read err:%s", err.Error()))
	}
	err = yaml.Unmarshal(b, conf)
	if err != nil {
		return errors.New(fmt.Sprintf("config file unmarshal err:%s", err.Error()))
	}
	return nil
}
