package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	MQTTUrl       string `mapstructure:"mqtt-url"`
	MQTTUsername  string `mapstructure:"mqtt-username"`
	MQTTPassword  string `mapstructure:"mqtt-password"`
	MQTTPrefix    string `mapstructure:"mqtt-prefix"`
	WiNetHost     string `mapstructure:"winet-host"`
	WiNetUsername string `mapstructure:"winet-username"`
	WiNetPassword string `mapstructure:"winet-password"`
	PollInterval  int    `mapstructure:"poll-interval"`
	HealthPort    int    `mapstructure:"health-port"`
}

func (c *Config) Validate() error {
	if c.WiNetHost == "" {
		return fmt.Errorf("winet-host is required")
	}
	if c.MQTTUrl == "" {
		return fmt.Errorf("mqtt-url is required")
	}
	if c.PollInterval < 1 || c.PollInterval > 3600 {
		return fmt.Errorf("poll-interval must be between 1 and 3600")
	}
	return nil
}

func SetDefaults() {
	viper.SetDefault("mqtt-prefix", "homeassistant")
	viper.SetDefault("winet-username", "admin")
	viper.SetDefault("winet-password", "pw8888")
	viper.SetDefault("poll-interval", 10)
	viper.SetDefault("health-port", 8081)
}

func BindEnv() {
	viper.SetEnvPrefix("WINET2MQTT")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}
