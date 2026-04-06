//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
)

// These tests require the docker-compose.test.yml stack to be running.
// Run: make integration-test

func mqttURL() string {
	if url := os.Getenv("TEST_MQTT_URL"); url != "" {
		return url
	}
	return "tcp://127.0.0.1:1883"
}

func healthURL() string {
	if url := os.Getenv("TEST_HEALTH_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:8081/healthz"
}

func subscribe(t *testing.T, topic string) (map[string][]byte, *sync.Mutex, pahomqtt.Client) {
	t.Helper()
	var mu sync.Mutex
	messages := make(map[string][]byte)

	opts := pahomqtt.NewClientOptions().
		AddBroker(mqttURL()).
		SetClientID(fmt.Sprintf("integration-test-%d", time.Now().UnixNano()%10000))
	client := pahomqtt.NewClient(opts)

	tok := client.Connect()
	if !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		t.Fatalf("MQTT connect: %v", tok.Error())
	}

	subTok := client.Subscribe(topic, 1, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		mu.Lock()
		messages[msg.Topic()] = append([]byte{}, msg.Payload()...)
		mu.Unlock()
	})
	if !subTok.WaitTimeout(5*time.Second) || subTok.Error() != nil {
		t.Fatalf("subscribe: %v", subTok.Error())
	}

	return messages, &mu, client
}

func waitForMessages(t *testing.T, mu *sync.Mutex, messages map[string][]byte, min int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(messages)
		mu.Unlock()
		if n >= min {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	mu.Lock()
	t.Fatalf("timeout: expected at least %d messages, got %d", min, len(messages))
	mu.Unlock() //nolint:govet
}

func TestEndToEnd_DeviceDiscovery(t *testing.T) {
	messages, mu, client := subscribe(t, "homeassistant/sensor/SH10RS_A2582008920/config")
	defer client.Disconnect(1000)

	waitForMessages(t, mu, messages, 1)

	mu.Lock()
	defer mu.Unlock()

	data := messages["homeassistant/sensor/SH10RS_A2582008920/config"]
	var reg map[string]interface{}
	if err := json.Unmarshal(data, &reg); err != nil {
		t.Fatalf("unmarshal device config: %v", err)
	}

	device, _ := reg["device"].(map[string]interface{})
	if device["model"] != "SH10RS" {
		t.Errorf("expected model SH10RS, got %v", device["model"])
	}
	if device["manufacturer"] != "Sungrow" {
		t.Errorf("expected manufacturer Sungrow, got %v", device["manufacturer"])
	}
}

func TestEndToEnd_SensorConfigs(t *testing.T) {
	messages, mu, client := subscribe(t, "homeassistant/sensor/SH10RS_A2582008920/+/config")
	defer client.Disconnect(1000)

	waitForMessages(t, mu, messages, 5)

	mu.Lock()
	defer mu.Unlock()

	cases := []struct {
		slug        string
		deviceClass string
		unit        string
		stateClass  string
	}{
		{"total_active_power", "power", "kW", "measurement"},
		{"daily_pv_yield", "energy", "kWh", "total_increasing"},
		{"ac_voltage", "voltage", "V", "measurement"},
		{"battery_level_soc", "battery", "%", "measurement"},
		{"internal_air_temperature", "temperature", "°C", "measurement"},
	}

	for _, tc := range cases {
		topic := fmt.Sprintf("homeassistant/sensor/SH10RS_A2582008920/%s/config", tc.slug)
		data, ok := messages[topic]
		if !ok {
			t.Errorf("missing config for %s", tc.slug)
			continue
		}

		var cfg map[string]interface{}
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Errorf("unmarshal %s config: %v", tc.slug, err)
			continue
		}

		if dc, _ := cfg["device_class"].(string); dc != tc.deviceClass {
			t.Errorf("%s: expected device_class %q, got %q", tc.slug, tc.deviceClass, dc)
		}
		if u, _ := cfg["unit_of_measurement"].(string); u != tc.unit {
			t.Errorf("%s: expected unit %q, got %q", tc.slug, tc.unit, u)
		}
		if sc, _ := cfg["state_class"].(string); sc != tc.stateClass {
			t.Errorf("%s: expected state_class %q, got %q", tc.slug, tc.stateClass, sc)
		}
	}
}

func TestEndToEnd_SensorValues(t *testing.T) {
	messages, mu, client := subscribe(t, "homeassistant/sensor/SH10RS_A2582008920/+/state")
	defer client.Disconnect(1000)

	waitForMessages(t, mu, messages, 5)

	mu.Lock()
	defer mu.Unlock()

	cases := []struct {
		slug  string
		value float64
		unit  string
	}{
		{"total_active_power", 3.5, "kW"},
		{"ac_voltage", 239.5, "V"},
		{"daily_pv_yield", 12.5, "kWh"},
		{"total_reactive_power", 1500, "var"},
	}

	for _, tc := range cases {
		topic := fmt.Sprintf("homeassistant/sensor/SH10RS_A2582008920/%s/state", tc.slug)
		data, ok := messages[topic]
		if !ok {
			t.Errorf("missing state for %s", tc.slug)
			continue
		}

		var state map[string]interface{}
		if err := json.Unmarshal(data, &state); err != nil {
			t.Errorf("unmarshal %s state: %v", tc.slug, err)
			continue
		}

		val, _ := state["value"].(float64)
		if val != tc.value {
			t.Errorf("%s: expected value %v, got %v", tc.slug, tc.value, val)
		}
		unit, _ := state["unit_of_measurement"].(string)
		if unit != tc.unit {
			t.Errorf("%s: expected unit %q, got %q", tc.slug, tc.unit, unit)
		}
	}

	// MPPT string sensors.
	mpptTopics := []string{
		"homeassistant/sensor/SH10RS_A2582008920/string_1_voltage/state",
		"homeassistant/sensor/SH10RS_A2582008920/string_1_power/state",
		"homeassistant/sensor/SH10RS_A2582008920/string_2_voltage/state",
		"homeassistant/sensor/SH10RS_A2582008920/mppt_total_power/state",
	}
	for _, topic := range mpptTopics {
		if _, ok := messages[topic]; !ok {
			t.Errorf("missing MPPT topic: %s", topic)
		}
	}

	// Text sensor.
	if data, ok := messages["homeassistant/sensor/SH10RS_A2582008920/running_status/state"]; ok {
		var state map[string]interface{}
		if err := json.Unmarshal(data, &state); err != nil {
			t.Errorf("unmarshal running_status: %v", err)
		} else if state["value"] != "Running" {
			t.Errorf("expected Running, got %v", state["value"])
		}
	} else {
		t.Error("missing running_status state")
	}
}

func TestEndToEnd_HealthCheck(t *testing.T) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL()) //nolint:gosec,noctx
		if err == nil {
			var health map[string]interface{}
			decodeErr := json.NewDecoder(resp.Body).Decode(&health)
			_ = resp.Body.Close()
			if decodeErr == nil && resp.StatusCode == 200 {
				if health["status"] != "ok" {
					t.Errorf("expected status ok, got %v", health["status"])
				}
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatal("health check never returned 200")
}
