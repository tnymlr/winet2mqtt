package mqtt

import (
	"testing"
)

func TestDeviceClasses(t *testing.T) {
	expected := map[string]string{
		"W":    "power",
		"kW":   "power",
		"V":    "voltage",
		"A":    "current",
		"kWh":  "energy",
		"Wh":   "energy",
		"℃":    "temperature",
		"°C":   "temperature",
		"kvar": "reactive_power",
		"var":  "reactive_power",
		"Hz":   "frequency",
		"%":    "battery",
	}

	for unit, want := range expected {
		got, ok := DeviceClasses[unit]
		if !ok {
			t.Errorf("DeviceClasses[%q] not found", unit)
			continue
		}
		if got != want {
			t.Errorf("DeviceClasses[%q] = %q, want %q", unit, got, want)
		}
	}

	// kΩ should NOT have a device class.
	if _, ok := DeviceClasses["kΩ"]; ok {
		t.Error("kΩ should not have a device class")
	}
}

func TestStateClasses(t *testing.T) {
	// All defined state classes should be "measurement".
	for unit, sc := range StateClasses {
		if sc != "measurement" {
			t.Errorf("StateClasses[%q] = %q, want measurement", unit, sc)
		}
	}

	// kWh should NOT be in StateClasses (it's handled as total_increasing).
	if _, ok := StateClasses["kWh"]; ok {
		t.Error("kWh should not be in StateClasses (handled as total_increasing)")
	}
}
