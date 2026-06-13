package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var Version = "1.0.0"
var localConfig = ".xposrc"
var remoteConfig = "https://xpos-io.com/config.json"

type Config struct {
	Remote struct {
		Domain string `json:"domain"`
		Events string `json:"events"`
	}
	Local struct {
		AuthToken string `yaml:"auth_token"`
	}
}

func (c *Config) Load() error {
	configDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting user config directory: %s", err)
	}

	filePath := filepath.Join(configDir, localConfig)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error: no auth token, request at https://xpos-io.com/auth")
	}

	if err := yaml.Unmarshal(data, &c.Local); err != nil {
		return fmt.Errorf("error unmarshaling config file contents: %s", err)
	}
	c.Remote.Domain = "localhost"
	c.Remote.Events = "localhost:9876"

	// response, err := http.Get(remoteConfig)
	// if err != nil || response.StatusCode != http.StatusOK {
	// 	return fmt.Errorf("error fetching %s: %s", remoteConfig, err)
	// }
	// defer response.Body.Close()

	// if err := json.NewDecoder(response.Body).Decode(&c.Remote); err != nil {
	// 	return fmt.Errorf("error decoding config file: %s", err)
	// }

	return nil
}

func (c *Config) Write() error {
	content, err := yaml.Marshal(c.Local)
	if err != nil {
		return fmt.Errorf("error marshaling config: %s", err)
	}

	configDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting user config directory: %s", err)
	}

	filePath := filepath.Join(configDir, localConfig)
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("error creating config file: %s", err)
	}

	if _, err = file.Write(content); err != nil {
		return fmt.Errorf("error writitng to config file: %s", err)
	}
	return nil
}
