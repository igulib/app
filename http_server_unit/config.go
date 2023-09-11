package http_server_unit

import "gopkg.in/yaml.v3"

// Config defines the configuration for the HttpServerUnit.
type Config struct {
	Host                 string `yaml:"host" json:"host" toml:"host"`
	Port                 string `yaml:"port" json:"port" toml:"port"`
	ShutdownPeriodMillis int64  `yaml:"shutdown_period_millis" json:"shutdown_period_millis" toml:"shutdown_period_millis"`
}

type validatedConfig struct {
	Host                 string
	Port                 string
	ShutdownPeriodMillis int64
}

func ParseYamlConfig(data []byte) (*Config, error) {
	config := &Config{}
	err := yaml.Unmarshal(data, config)
	return config, err
}

// func (c *Config) ValidateConfig() error {
// 	c.validatedHost = c.Host
// 	if c.validatedHost == "" {
// 		c.validatedPort = "localhost"
// 	}
// 	c.validatedPort = c.Port
// 	if c.validatedPort == "" {
// 		c.validatedPort = "8080"
// 	}
// 	if c.ShutdownPeriodMillis == 0 {
// 		c.validatedShutdownPeriodMillis = 5000
// 	} else {
// 		c.validatedShutdownPeriodMillis = c.ShutdownPeriodMillis
// 	}
// 	return nil
// }

func (c *Config) ValidateConfig() (*validatedConfig, error) {
	vc := &validatedConfig{}
	if c.Host == "" {
		c.Host = "localhost"
	}
	vc.Host = c.Host

	if c.Port == "" {
		c.Port = "8080"
	}
	vc.Port = c.Port

	if c.ShutdownPeriodMillis == 0 {
		c.ShutdownPeriodMillis = 5000
	}
	vc.ShutdownPeriodMillis = c.ShutdownPeriodMillis

	return vc, nil
}
