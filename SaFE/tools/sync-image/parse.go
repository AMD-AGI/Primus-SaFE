package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	templateConfigPath = "."
	templateConfigName = "config.yaml"
)

func parseTemplateConfig() (*syncOptions, error) {
	configPath := fmt.Sprintf("%s/%s", templateConfigPath, templateConfigName)

	var config = &syncOptions{}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	expandedYaml := os.ExpandEnv(string(data))

	err = yaml.Unmarshal([]byte(expandedYaml), &config)
	if err != nil {
		logrus.Errorf("Error parsing YAML: %v", err)
		return nil, err
	}

	return config, nil
}

func parseStr(args []string) []string {
	var result []string
	for _, arg := range args {
		arg = strings.ReplaceAll(arg, "\n", "")
		arg = strings.ReplaceAll(arg, "\t", "")
		arg = strings.TrimSpace(arg)
		result = append(result, arg)
	}
	return result
}
