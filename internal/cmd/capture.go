package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/spf13/cobra"

	"winet2mqtt/internal/snapshot"
)

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Capture current MQTT state and dump as JSON",
	Long:  "Subscribes to the MQTT broker, collects all retained config and state messages for Sungrow devices, and outputs a JSON snapshot for comparison.",
	RunE:  runCapture,
}

func init() {
	f := captureCmd.Flags()
	f.String("mqtt-url", "", "MQTT broker URL (required)")
	f.String("mqtt-username", "", "MQTT broker username")
	f.String("mqtt-password", "", "MQTT broker password")
	f.String("mqtt-prefix", "homeassistant", "MQTT topic prefix")
	f.Int("timeout", 10, "Seconds to wait for messages")
	f.StringP("output", "o", "", "Output file (default: stdout)")
	f.String("device-filter", "", "Only capture devices matching this slug prefix (e.g. SH10RS)")

	rootCmd.AddCommand(captureCmd)
}

// capturedConfig holds parsed HA discovery config messages.
type capturedConfig struct {
	Name              string `json:"name"`
	UniqueID          string `json:"unique_id"`
	StateTopic        string `json:"state_topic"`
	UnitOfMeasurement string `json:"unit_of_measurement,omitempty"`
	StateClass        string `json:"state_class,omitempty"`
	DeviceClass       string `json:"device_class,omitempty"`
	Encoding          string `json:"encoding,omitempty"`
	ValueTemplate     string `json:"value_template,omitempty"`
	Device            struct {
		Name         string   `json:"name"`
		Identifiers  []string `json:"identifiers"`
		Model        string   `json:"model"`
		Manufacturer string   `json:"manufacturer,omitempty"`
	} `json:"device"`
	// Fields for top-level device registration.
	Abbreviation string `json:"~,omitempty"`
}

// capturedState holds a parsed state message.
type capturedState struct {
	Value             interface{} `json:"value"`
	UnitOfMeasurement string      `json:"unit_of_measurement,omitempty"`
}

func runCapture(cmd *cobra.Command, _ []string) error {
	bindFlags(cmd)

	mqttURL, _ := cmd.Flags().GetString("mqtt-url")
	mqttUser, _ := cmd.Flags().GetString("mqtt-username")
	mqttPass, _ := cmd.Flags().GetString("mqtt-password")
	prefix, _ := cmd.Flags().GetString("mqtt-prefix")
	timeoutSec, _ := cmd.Flags().GetInt("timeout")
	output, _ := cmd.Flags().GetString("output")
	deviceFilter, _ := cmd.Flags().GetString("device-filter")

	// Collect messages.
	configs := make(map[string]*capturedConfig)       // topic -> config
	states := make(map[string]*capturedState)         // topic -> state
	deviceConfigs := make(map[string]*capturedConfig) // device slug -> device config

	opts := pahomqtt.NewClientOptions().
		AddBroker(mqttURL).
		SetClientID(fmt.Sprintf("winet2mqtt-capture-%d", time.Now().UnixNano()%10000)).
		SetCleanSession(true)

	if mqttUser != "" {
		opts.SetUsername(mqttUser)
	}
	if mqttPass != "" {
		opts.SetPassword(mqttPass)
	}

	client := pahomqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("MQTT connect timeout")
	}
	if token.Error() != nil {
		return fmt.Errorf("MQTT connect: %w", token.Error())
	}
	defer client.Disconnect(1000)

	// Subscribe to all sensor topics under prefix.
	topic := fmt.Sprintf("%s/sensor/#", prefix)
	client.Subscribe(topic, 1, func(_ pahomqtt.Client, msg pahomqtt.Message) {
		t := msg.Topic()
		payload := msg.Payload()

		rel := strings.TrimPrefix(t, prefix+"/sensor/")
		parts := strings.Split(rel, "/")

		// Filter by device slug prefix if specified.
		if deviceFilter != "" && (len(parts) == 0 || !strings.HasPrefix(parts[0], deviceFilter)) {
			return
		}

		if len(parts) == 2 && parts[1] == "config" {
			// Device-level config: prefix/sensor/{device_slug}/config
			var cfg capturedConfig
			if err := json.Unmarshal(payload, &cfg); err == nil {
				deviceConfigs[parts[0]] = &cfg
			}
		} else if len(parts) == 3 && parts[2] == "config" {
			// Sensor config: prefix/sensor/{device_slug}/{sensor_slug}/config
			var cfg capturedConfig
			if err := json.Unmarshal(payload, &cfg); err == nil {
				key := parts[0] + "/" + parts[1]
				configs[key] = &cfg
			}
		} else if len(parts) == 3 && parts[2] == "state" {
			// Sensor state: prefix/sensor/{device_slug}/{sensor_slug}/state
			var st capturedState
			if err := json.Unmarshal(payload, &st); err == nil {
				key := parts[0] + "/" + parts[1]
				states[key] = &st
			}
		}
	})

	fmt.Fprintf(os.Stderr, "Listening for %d seconds on %s ...\n", timeoutSec, topic)
	time.Sleep(time.Duration(timeoutSec) * time.Second)

	// Build snapshot.
	snap := &snapshot.Snapshot{}

	for slug, cfg := range deviceConfigs {
		model := cfg.Device.Model
		serial := ""
		// Device slug is typically MODEL_SERIAL.
		if idx := strings.Index(slug, "_"); idx > 0 {
			serial = slug[idx+1:]
		}
		snap.Devices = append(snap.Devices, snapshot.DeviceEntry{
			DeviceSlug:   slug,
			Model:        model,
			Serial:       serial,
			Name:         cfg.Device.Name,
			Manufacturer: cfg.Device.Manufacturer,
		})
	}

	for key, cfg := range configs {
		parts := strings.SplitN(key, "/", 2)
		deviceSlug := parts[0]
		sensorSlug := parts[1]

		value := ""
		if st, ok := states[key]; ok {
			switch v := st.Value.(type) {
			case float64:
				value = fmt.Sprintf("%g", v)
			case string:
				value = v
			default:
				value = fmt.Sprintf("%v", v)
			}
		}

		isNumeric := cfg.Encoding != "utf-8" && cfg.UnitOfMeasurement != ""

		snap.Sensors = append(snap.Sensors, snapshot.SensorEntry{
			DeviceSlug:  deviceSlug,
			SensorSlug:  sensorSlug,
			SensorName:  cfg.Name,
			Value:       value,
			Unit:        cfg.UnitOfMeasurement,
			DeviceClass: cfg.DeviceClass,
			StateClass:  cfg.StateClass,
			IsNumeric:   isNumeric,
		})
	}

	data, err := snap.JSON()
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	if output != "" {
		if err := os.WriteFile(output, data, 0600); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote %d devices, %d sensors to %s\n", len(snap.Devices), len(snap.Sensors), output)
	} else {
		fmt.Println(string(data))
	}

	return nil
}
