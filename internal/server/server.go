package server

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"winet2mqtt/internal/config"
	"winet2mqtt/internal/health"
	"winet2mqtt/internal/mqtt"
	"winet2mqtt/internal/winet"
)

// Server ties together the WiNet client, MQTT publisher, and health checks.
type Server struct {
	cfg       *config.Config
	logger    *slog.Logger
	publisher *mqtt.Publisher
	client    *winet.Client
	health    *health.Server
	connected atomic.Bool
}

// New creates a new Server from the given config.
func New(cfg *config.Config, logger *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		logger: logger,
	}
}

// Run starts all components and blocks until the context is cancelled.
func (s *Server) Run(ctx context.Context) error {
	// Fetch i18n properties from the dongle.
	s.logger.Info("fetching i18n properties", "host", s.cfg.WiNetHost)
	props, ssl, err := winet.FetchProperties(ctx, s.logger, s.cfg.WiNetHost, "en_US", false)
	if err != nil {
		return fmt.Errorf("fetch properties: %w", err)
	}
	s.logger.Info("fetched properties", "count", len(props), "ssl", ssl)

	// Connect to MQTT.
	s.logger.Info("connecting to MQTT", "url", s.cfg.MQTTUrl)
	s.publisher, err = mqtt.NewPublisher(s.logger, s.cfg.MQTTUrl, s.cfg.MQTTUsername, s.cfg.MQTTPassword, s.cfg.MQTTPrefix)
	if err != nil {
		return fmt.Errorf("connect MQTT: %w", err)
	}
	defer s.publisher.Close()

	// Create WiNet client.
	s.client = winet.NewClient(
		s.cfg.WiNetHost,
		s.cfg.WiNetUsername,
		s.cfg.WiNetPassword,
		s.cfg.PollInterval,
		ssl,
		props,
		s.logger,
		s.onDeviceUpdate,
	)

	// Set up health checks.
	s.health = health.NewServer(s.logger, s.cfg.HealthPort)
	s.health.Register("mqtt", s.publisher)
	s.health.Register("winet", health.CheckerFunc(func(_ context.Context) error {
		if !s.connected.Load() {
			return fmt.Errorf("WiNet client not connected")
		}
		return nil
	}))

	// Health server (non-fatal if it fails).
	go func() {
		if err := s.health.Start(); err != nil {
			s.logger.Warn("health server failed (non-fatal)", "error", err)
		}
	}()

	// Run WiNet client (blocks until context cancelled or fatal error).
	s.connected.Store(true)
	err = s.client.Run(ctx)
	s.connected.Store(false)

	// Graceful shutdown.
	s.logger.Info("shutting down")
	s.client.Close()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if shutdownErr := s.health.Shutdown(shutdownCtx); shutdownErr != nil {
		s.logger.Warn("health server shutdown error", "error", shutdownErr)
	}

	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("winet client: %w", err)
	}
	return nil
}

func (s *Server) onDeviceUpdate(devices []winet.DeviceData) {
	s.connected.Store(true)

	var readings []mqtt.SensorReading
	for _, dev := range devices {
		for _, sensor := range dev.Sensors {
			sensorSlug := mqtt.MakeSensorSlug(sensor.Name)
			_, isNumeric := isNumericValue(sensor.Value, sensor.Unit)

			readings = append(readings, mqtt.SensorReading{
				DeviceSerial: dev.Device.DevSN,
				DeviceModel:  dev.Device.DevModel,
				DeviceName:   dev.Device.DevName,
				SensorName:   sensor.Name,
				SensorSlug:   sensorSlug,
				Value:        sensor.Value,
				Unit:         sensor.Unit,
				IsNumeric:    isNumeric,
			})
		}
	}

	s.publisher.PublishAll(readings)
	s.logger.Info("published sensor data", "devices", len(devices), "readings", len(readings))
}

// isNumericValue checks if a sensor value is numeric based on the unit and actual value.
func isNumericValue(value, unit string) (float64, bool) {
	if unit == "" {
		return 0, false
	}
	if _, ok := winet.NumericUnits[unit]; !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
