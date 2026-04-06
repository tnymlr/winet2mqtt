package cmd

import (
	"fmt"
	"os"

	"winet2mqtt/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "winet2mqtt",
	Short: "Bridge between Sungrow WiNet dongles and MQTT",
	Long:  "winet2mqtt connects to Sungrow WiNet-S/S2 solar inverter dongles via WebSocket and publishes sensor data to an MQTT broker.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	config.SetDefaults()
	config.BindEnv()
}

func bindFlags(cmd *cobra.Command) {
	_ = viper.BindPFlags(cmd.Flags())
}

func loadConfig() (*config.Config, error) {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}
