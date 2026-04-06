package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"winet2mqtt/internal/server"

	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the winet2mqtt bridge server",
	Long:  "Connects to a WiNet dongle, polls sensor data, and publishes to MQTT.",
	RunE:  runServer,
}

func init() {
	f := serverCmd.Flags()
	f.String("winet-host", "", "WiNet dongle IP or hostname (required)")
	f.String("winet-username", "admin", "WiNet dongle username")
	f.String("winet-password", "pw8888", "WiNet dongle password")
	f.String("mqtt-url", "", "MQTT broker URL, e.g. tcp://localhost:1883 (required)")
	f.String("mqtt-username", "", "MQTT broker username")
	f.String("mqtt-password", "", "MQTT broker password")
	f.String("mqtt-prefix", "homeassistant", "MQTT topic prefix (HA discovery)")
	f.Int("poll-interval", 10, "Polling interval in seconds (1-3600)")
	f.Int("health-port", 8081, "Health check HTTP port")

	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, _ []string) error {
	bindFlags(cmd)

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("starting winet2mqtt",
		"winet_host", cfg.WiNetHost,
		"mqtt_url", cfg.MQTTUrl,
		"mqtt_prefix", cfg.MQTTPrefix,
		"poll_interval", cfg.PollInterval,
		"health_port", cfg.HealthPort,
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	srv := server.New(cfg, logger)
	return srv.Run(ctx)
}
