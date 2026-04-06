package snapshot

import (
	"encoding/json"
	"testing"
)

func TestSnapshot_Sort(t *testing.T) { //nolint:goconst
	snap := &Snapshot{
		Devices: []DeviceEntry{
			{DeviceSlug: "B_device"},
			{DeviceSlug: "A_device"},
		},
		Sensors: []SensorEntry{
			{DeviceSlug: "A_device", SensorSlug: "z_sensor"},
			{DeviceSlug: "B_device", SensorSlug: "a_sensor"},
			{DeviceSlug: "A_device", SensorSlug: "a_sensor"},
		},
	}

	snap.Sort()

	if snap.Devices[0].DeviceSlug != "A_device" {
		t.Errorf("devices not sorted: first is %s", snap.Devices[0].DeviceSlug)
	}
	if snap.Sensors[0].DeviceSlug != "A_device" || snap.Sensors[0].SensorSlug != "a_sensor" {
		t.Errorf("sensors not sorted: first is %s/%s", snap.Sensors[0].DeviceSlug, snap.Sensors[0].SensorSlug)
	}
	if snap.Sensors[1].DeviceSlug != "A_device" || snap.Sensors[1].SensorSlug != "z_sensor" {
		t.Errorf("sensors not sorted: second is %s/%s", snap.Sensors[1].DeviceSlug, snap.Sensors[1].SensorSlug)
	}
	if snap.Sensors[2].DeviceSlug != "B_device" {
		t.Errorf("sensors not sorted: third is %s/%s", snap.Sensors[2].DeviceSlug, snap.Sensors[2].SensorSlug)
	}
}

func TestSnapshot_JSON(t *testing.T) {
	snap := &Snapshot{
		Devices: []DeviceEntry{
			{DeviceSlug: "SH10RS_123", Model: "SH10RS", Serial: "123", Name: "SH10RS 123"},
		},
		Sensors: []SensorEntry{
			{DeviceSlug: "SH10RS_123", SensorSlug: "voltage", SensorName: "Voltage", Value: "240", Unit: "V", IsNumeric: true},
		},
	}

	data, err := snap.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Verify structure.
	devices, ok := parsed["devices"].([]interface{})
	if !ok || len(devices) != 1 {
		t.Error("expected 1 device in output")
	}
	sensors, ok := parsed["sensors"].([]interface{})
	if !ok || len(sensors) != 1 {
		t.Error("expected 1 sensor in output")
	}
}

func TestSnapshot_JSON_Empty(t *testing.T) {
	snap := &Snapshot{}
	data, err := snap.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}
	if string(data) == "" {
		t.Error("expected non-empty JSON")
	}
}
