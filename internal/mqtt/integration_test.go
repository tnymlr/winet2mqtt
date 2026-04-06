package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startMosquitto(t *testing.T) (string, func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "eclipse-mosquitto:2", //nolint:misspell
		ExposedPorts: []string{"1883/tcp"},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader("listener 1883\nallow_anonymous true\n"),
				ContainerFilePath: "/mosquitto/config/mosquitto.conf", //nolint:misspell
			},
		},
		WaitingFor: wait.ForListeningPort("1883/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("start mosquitto: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get host: %v", err)
	}
	port, err := container.MappedPort(ctx, "1883/tcp")
	if err != nil {
		t.Fatalf("get port: %v", err)
	}

	url := fmt.Sprintf("tcp://%s:%s", host, port.Port())
	cleanup := func() { _ = container.Terminate(ctx) }
	return url, cleanup
}

func TestPublisher_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	mqttURL, cleanup := startMosquitto(t)
	defer cleanup()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	pub, err := NewPublisher(logger, mqttURL, "", "", "homeassistant")
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer pub.Close()

	if !pub.IsConnected() {
		t.Fatal("publisher not connected")
	}

	// Subscribe to capture published messages.
	var mu sync.Mutex
	messages := make(map[string][]byte)

	subOpts := pahomqtt.NewClientOptions().
		AddBroker(mqttURL).
		SetClientID("test-subscriber")
	subClient := pahomqtt.NewClient(subOpts)
	token := subClient.Connect()
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatalf("subscribe connect: %v", token.Error())
	}
	defer subClient.Disconnect(500)

	subClient.Subscribe("homeassistant/sensor/#", 1, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		mu.Lock()
		messages[msg.Topic()] = msg.Payload()
		mu.Unlock()
	})

	// Poll until subscription is active by publishing and checking.
	waitForSubscription(t, subClient, pub)

	// Publish sensor data.
	readings := []SensorReading{
		{
			DeviceSerial: "TEST123", DeviceModel: "SH10RS", DeviceName: "SH10RS TEST123",
			SensorName: "Total active power", SensorSlug: "total_active_power",
			Value: "3.5", Unit: "kW", IsNumeric: true,
		},
		{
			DeviceSerial: "TEST123", DeviceModel: "SH10RS", DeviceName: "SH10RS TEST123",
			SensorName: "Running status", SensorSlug: "running_status",
			Value: "Running", Unit: "", IsNumeric: false,
		},
		{
			DeviceSerial: "TEST123", DeviceModel: "SH10RS", DeviceName: "SH10RS TEST123",
			SensorName: "Total power factor", SensorSlug: "total_power_factor",
			Value: "0.95", Unit: "", IsNumeric: false,
		},
	}

	pub.PublishAll(readings)

	// Poll until we have all expected messages: 1 device config + 3 sensor configs + 3 states = 7.
	waitForMessages(t, &mu, messages, 7)

	mu.Lock()
	defer mu.Unlock()

	// Check device registration.
	deviceConfigTopic := "homeassistant/sensor/SH10RS_TEST123/config"
	if data, ok := messages[deviceConfigTopic]; !ok {
		t.Error("missing device config message")
	} else {
		var reg DeviceRegistration
		if err := json.Unmarshal(data, &reg); err != nil {
			t.Fatalf("unmarshal device config: %v", err)
		}
		if reg.Device.Model != "SH10RS" {
			t.Errorf("expected model SH10RS, got %s", reg.Device.Model)
		}
		if reg.Device.Manufacturer != "Sungrow" {
			t.Errorf("expected manufacturer Sungrow, got %s", reg.Device.Manufacturer)
		}
	}

	// Check numeric sensor config.
	powerConfigTopic := "homeassistant/sensor/SH10RS_TEST123/total_active_power/config"
	if data, ok := messages[powerConfigTopic]; !ok {
		t.Error("missing power sensor config")
	} else {
		var cfg ConfigPayload
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if cfg.DeviceClass != "power" {
			t.Errorf("expected device_class power, got %s", cfg.DeviceClass)
		}
		if cfg.StateClass != "measurement" {
			t.Errorf("expected state_class measurement, got %s", cfg.StateClass)
		}
		if cfg.UnitOfMeasurement != "kW" {
			t.Errorf("expected unit kW, got %s", cfg.UnitOfMeasurement)
		}
		if cfg.ValueTemplate != "{{ value_json.value | float }}" {
			t.Errorf("expected float template, got %s", cfg.ValueTemplate)
		}
	}

	// Check text sensor config.
	statusConfigTopic := "homeassistant/sensor/SH10RS_TEST123/running_status/config"
	if data, ok := messages[statusConfigTopic]; !ok {
		t.Error("missing status sensor config")
	} else {
		var cfg ConfigPayload
		_ = json.Unmarshal(data, &cfg)
		if cfg.Encoding != "utf-8" {
			t.Errorf("expected encoding utf-8, got %s", cfg.Encoding)
		}
		if cfg.ValueTemplate != "{{ value_json.value }}" {
			t.Errorf("expected value template, got %s", cfg.ValueTemplate)
		}
	}

	// Check power factor special case.
	pfConfigTopic := "homeassistant/sensor/SH10RS_TEST123/total_power_factor/config"
	if data, ok := messages[pfConfigTopic]; !ok {
		t.Error("missing power factor sensor config")
	} else {
		var cfg ConfigPayload
		_ = json.Unmarshal(data, &cfg)
		if cfg.DeviceClass != "power_factor" {
			t.Errorf("expected device_class power_factor, got %s", cfg.DeviceClass)
		}
		if cfg.UnitOfMeasurement != " " {
			t.Errorf("expected unit ' ' (space), got %q", cfg.UnitOfMeasurement)
		}
	}

	// Check numeric state payload.
	powerStateTopic := "homeassistant/sensor/SH10RS_TEST123/total_active_power/state"
	if data, ok := messages[powerStateTopic]; !ok {
		t.Error("missing power state message")
	} else {
		var state map[string]interface{}
		_ = json.Unmarshal(data, &state)
		if val, ok := state["value"].(float64); !ok || val != 3.5 {
			t.Errorf("expected value 3.5, got %v", state["value"])
		}
		if unit, ok := state["unit_of_measurement"].(string); !ok || unit != "kW" {
			t.Errorf("expected unit kW, got %v", state["unit_of_measurement"])
		}
	}

	// Check text state payload.
	statusStateTopic := "homeassistant/sensor/SH10RS_TEST123/running_status/state"
	if data, ok := messages[statusStateTopic]; !ok {
		t.Error("missing status state message")
	} else {
		var state map[string]interface{}
		_ = json.Unmarshal(data, &state)
		if val, ok := state["value"].(string); !ok || val != "Running" {
			t.Errorf("expected value 'Running', got %v", state["value"])
		}
	}
}

func TestPublisher_Healthy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	mqttURL, cleanup := startMosquitto(t)
	defer cleanup()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	pub, err := NewPublisher(logger, mqttURL, "", "", "test")
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}

	if err := pub.Healthy(context.Background()); err != nil {
		t.Errorf("expected healthy, got: %v", err)
	}

	pub.Close()
	time.Sleep(100 * time.Millisecond)
}

func TestPublisher_UnitNormalization(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	mqttURL, cleanup := startMosquitto(t)
	defer cleanup()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	pub, err := NewPublisher(logger, mqttURL, "", "", "homeassistant")
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer pub.Close()

	// Subscribe.
	var mu sync.Mutex
	messages := make(map[string][]byte)
	subOpts := pahomqtt.NewClientOptions().AddBroker(mqttURL).SetClientID(fmt.Sprintf("test-norm-%d", time.Now().UnixNano()%10000))
	subClient := pahomqtt.NewClient(subOpts)
	tok := subClient.Connect()
	if !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("subscriber connect: %v", tok.Error())
	}
	defer subClient.Disconnect(500)
	subTok := subClient.Subscribe("homeassistant/sensor/#", 1, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		mu.Lock()
		messages[msg.Topic()] = msg.Payload()
		mu.Unlock()
	})
	if !subTok.WaitTimeout(5*time.Second) || subTok.Error() != nil {
		t.Fatalf("subscribe: %v", subTok.Error())
	}

	// kvar → var with 1000x multiplier.
	// Use a fresh publisher so registeredDevices/registeredSensors are clean,
	// ensuring config messages are sent alongside state.
	pub2, err := NewPublisher(logger, mqttURL, "", "", "homeassistant")
	if err != nil {
		t.Fatalf("new publisher2: %v", err)
	}
	defer pub2.Close()

	// Publish repeatedly until subscription catches up.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// Reset registration state so configs get re-published.
		pub2.registeredDevices = make(map[string]bool)
		pub2.registeredSensors = make(map[string]bool)
		pub2.PublishAll([]SensorReading{{
			DeviceSerial: "T1", DeviceModel: "M1", DeviceName: "M1 T1",
			SensorName: "Reactive power", SensorSlug: "reactive_power",
			Value: "1.5", Unit: "kvar", IsNumeric: true,
		}})
		time.Sleep(200 * time.Millisecond)
		mu.Lock()
		n := len(messages)
		mu.Unlock()
		if n >= 3 {
			break
		}
	}

	mu.Lock()
	defer mu.Unlock()

	stateTopic := "homeassistant/sensor/M1_T1/reactive_power/state"
	if data, ok := messages[stateTopic]; ok {
		var state map[string]interface{}
		_ = json.Unmarshal(data, &state)
		// 1.5 kvar → 1500 var
		if val, ok := state["value"].(float64); !ok || val != 1500 {
			t.Errorf("expected 1500 var, got %v", state["value"])
		}
		if unit, ok := state["unit_of_measurement"].(string); !ok || unit != "var" {
			t.Errorf("expected unit var, got %v", state["unit_of_measurement"])
		}
	} else {
		t.Error("missing reactive power state")
	}
}

// waitForMessages polls until the message map has at least n entries, or times out.
func waitForMessages(t *testing.T, mu *sync.Mutex, messages map[string][]byte, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(messages)
		mu.Unlock()
		if count >= n {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	t.Fatalf("timeout: expected %d messages, got %d", n, len(messages))
	mu.Unlock()
}

// waitForSubscription ensures the MQTT subscription is active by publishing
// a probe message and polling until it's received.
func waitForSubscription(t *testing.T, sub pahomqtt.Client, pub *Publisher) {
	t.Helper()
	probe := make(chan struct{}, 1)
	probeTopic := fmt.Sprintf("homeassistant/sensor/_probe_%d/config", time.Now().UnixNano())
	sub.Subscribe(probeTopic, 1, func(_ pahomqtt.Client, _ pahomqtt.Message) {
		select {
		case probe <- struct{}{}:
		default:
		}
	})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pub.client.Publish(probeTopic, 0, false, "ping")
		select {
		case <-probe:
			sub.Unsubscribe(probeTopic)
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatal("timeout waiting for MQTT subscription to become active")
}
