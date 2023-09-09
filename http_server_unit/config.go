package http_server_unit

import "gopkg.in/yaml.v2"

// Config defines the configuration for the gin_unit
type Config struct {
	Host                          string `yaml:"host" json:"host" toml:"host"`
	validatedHost                 string
	Port                          string `yaml:"port" json:"port" toml:"port"`
	validatedPort                 string
	AssetsDir                     string `yaml:"assets_dir" json:"assets_dir" toml:"assets_dir"`
	validatedAssetsDir            string
	ShutdownPeriodMillis          int64 `yaml:"shutdown_period_millis" json:"shutdown_period_millis" toml:"shutdown_period_millis"`
	validatedShutdownPeriodMillis int64
}

func ParseYamlConfig(data []byte) (*Config, error) {
	config := &Config{}
	err := yaml.Unmarshal(data, config)
	return config, err
}

func (c *Config) ValidateConfig() error {
	c.validatedAssetsDir = c.AssetsDir
	c.validatedHost = c.Host
	c.validatedPort = c.Port
	if c.ShutdownPeriodMillis == 0 {
		c.validatedShutdownPeriodMillis = 5000
	} else {
		c.validatedShutdownPeriodMillis = c.ShutdownPeriodMillis
	}
	return nil
}
