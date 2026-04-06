package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gosimple/slug"
)

// SensorReading represents a single sensor value to publish.
type SensorReading struct {
	DeviceSerial string
	DeviceModel  string
	DeviceName   string
	SensorName   string
	SensorSlug   string
	Value        string
	Unit         string
	IsNumeric    bool
}

// Publisher publishes sensor data to an MQTT broker using HA MQTT discovery.
type Publisher struct {
	client            pahomqtt.Client
	prefix            string
	logger            *slog.Logger
	qos               byte
	registeredDevices map[string]bool
	registeredSensors map[string]bool
}

// NewPublisher creates a new MQTT publisher.
func NewPublisher(logger *slog.Logger, mqttURL, username, password, prefix string) (*Publisher, error) {
	opts := pahomqtt.NewClientOptions().
		AddBroker(mqttURL).
		SetClientID(fmt.Sprintf("winet2mqtt-%d", time.Now().UnixNano()%10000)).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetCleanSession(true)

	if username != "" {
		opts.SetUsername(username)
	}
	if password != "" {
		opts.SetPassword(password)
	}

	opts.SetOnConnectHandler(func(_ pahomqtt.Client) {
		logger.Info("connected to MQTT broker")
	})

	opts.SetConnectionLostHandler(func(_ pahomqtt.Client, err error) {
		logger.Warn("MQTT connection lost", "error", err)
	})

	client := pahomqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(15 * time.Second) {
		return nil, fmt.Errorf("MQTT connect timeout")
	}
	if token.Error() != nil {
		return nil, fmt.Errorf("MQTT connect: %w", token.Error())
	}

	return &Publisher{
		client:            client,
		prefix:            prefix,
		logger:            logger,
		qos:               1,
		registeredDevices: make(map[string]bool),
		registeredSensors: make(map[string]bool),
	}, nil
}

// EnsureDevice publishes the HA device registration config if not already registered.
func (p *Publisher) EnsureDevice(serial, model, name string) {
	deviceSlug := model + "_" + serial
	if p.registeredDevices[deviceSlug] {
		return
	}

	topic := fmt.Sprintf("%s/sensor/%s/config", p.prefix, deviceSlug)
	basePath := fmt.Sprintf("%s/sensor/%s", p.prefix, deviceSlug)

	reg := DeviceRegistration{
		Abbreviation: basePath,
		Name:         model + " " + serial,
		UniqueID:     strings.ToLower(deviceSlug),
		StateTopic:   "~/state",
		Device: DeviceConfig{
			Name:         model + " " + serial,
			Identifiers:  []string{deviceSlug},
			Model:        model,
			Manufacturer: "Sungrow",
		},
	}

	data, err := json.Marshal(reg)
	if err != nil {
		p.logger.Error("marshal device registration", "error", err)
		return
	}

	p.publish(topic, data, true)
	p.registeredDevices[deviceSlug] = true
	p.logger.Info("registered device", "slug", deviceSlug)
}

// EnsureSensor publishes the HA sensor config if not already registered.
func (p *Publisher) EnsureSensor(reading SensorReading) {
	deviceSlug := reading.DeviceModel + "_" + reading.DeviceSerial
	sensorKey := deviceSlug + "/" + reading.SensorSlug
	if p.registeredSensors[sensorKey] {
		return
	}

	stateTopic := fmt.Sprintf("%s/sensor/%s/%s/state", p.prefix, deviceSlug, reading.SensorSlug)
	configTopic := fmt.Sprintf("%s/sensor/%s/%s/config", p.prefix, deviceSlug, reading.SensorSlug)
	uniqueID := strings.ToLower(deviceSlug + "_" + reading.SensorSlug)

	unit, _ := NormalizeUnit(reading.Unit)

	cfg := ConfigPayload{
		Name:       reading.SensorName,
		StateTopic: stateTopic,
		UniqueID:   uniqueID,
		Device: DeviceConfig{
			Name:        reading.DeviceModel + " " + reading.DeviceSerial,
			Identifiers: []string{deviceSlug},
			Model:       reading.DeviceModel,
		},
	}

	if reading.IsNumeric {
		cfg.ValueTemplate = "{{ value_json.value | float }}"
		cfg.UnitOfMeasurement = unit

		if sc, ok := StateClasses[unit]; ok {
			cfg.StateClass = sc
		}
		if unit == "kWh" || unit == "Wh" {
			cfg.StateClass = "total_increasing"
		}
		if dc, ok := DeviceClasses[unit]; ok {
			cfg.DeviceClass = dc
		}
	} else {
		cfg.ValueTemplate = "{{ value_json.value }}"
		cfg.Encoding = "utf-8"
	}

	// Power factor: WiNet returns empty unit, but HA needs the metadata.
	if reading.SensorSlug == "total_power_factor" {
		cfg.DeviceClass = "power_factor"
		cfg.UnitOfMeasurement = " "
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		p.logger.Error("marshal sensor config", "error", err)
		return
	}

	p.publish(configTopic, data, true)
	p.registeredSensors[sensorKey] = true
}

// PublishState publishes a sensor state value.
func (p *Publisher) PublishState(reading SensorReading) {
	deviceSlug := reading.DeviceModel + "_" + reading.DeviceSerial
	topic := fmt.Sprintf("%s/sensor/%s/%s/state", p.prefix, deviceSlug, reading.SensorSlug)

	var payload []byte
	var err error

	if reading.IsNumeric {
		unit, multiplier := NormalizeUnit(reading.Unit)
		val, parseErr := strconv.ParseFloat(reading.Value, 64)
		if parseErr != nil {
			p.logger.Warn("non-numeric value for numeric sensor", "sensor", reading.SensorName, "value", reading.Value)
			return
		}
		val *= multiplier
		payload, err = json.Marshal(map[string]interface{}{
			"value":               val,
			"unit_of_measurement": unit,
		})
	} else {
		payload, err = json.Marshal(map[string]interface{}{
			"value": reading.Value,
		})
	}

	if err != nil {
		p.logger.Error("marshal state payload", "error", err)
		return
	}

	p.publish(topic, payload, false)
}

// PublishAll registers devices/sensors as needed and publishes all state values.
func (p *Publisher) PublishAll(readings []SensorReading) {
	for i := range readings {
		p.EnsureDevice(readings[i].DeviceSerial, readings[i].DeviceModel, readings[i].DeviceName)
		p.EnsureSensor(readings[i])
		p.PublishState(readings[i])
	}
}

func (p *Publisher) publish(topic string, payload []byte, retained bool) {
	token := p.client.Publish(topic, p.qos, retained, payload)
	go func() {
		if !token.WaitTimeout(5 * time.Second) {
			p.logger.Warn("MQTT publish timeout", "topic", topic)
		} else if token.Error() != nil {
			p.logger.Error("MQTT publish error", "topic", topic, "error", token.Error())
		}
	}()
}

// IsConnected returns whether the MQTT client is currently connected.
func (p *Publisher) IsConnected() bool {
	return p.client.IsConnected()
}

// Close disconnects from the MQTT broker.
func (p *Publisher) Close() {
	p.client.Disconnect(1000)
}

// Healthy returns nil if the publisher is connected, error otherwise.
func (p *Publisher) Healthy(_ context.Context) error {
	if !p.client.IsConnected() {
		return fmt.Errorf("MQTT client not connected")
	}
	return nil
}

// MakeSensorSlug creates a URL-safe slug from a sensor name.
// Uses underscores as separator to match Home Assistant conventions.
func MakeSensorSlug(name string) string {
	s := slug.Make(name)
	return strings.ReplaceAll(s, "-", "_")
}
