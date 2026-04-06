package snapshot

import (
	"encoding/json"
	"sort"
)

// Snapshot is the common output format for both capture and poll commands.
type Snapshot struct {
	Devices []DeviceEntry `json:"devices"`
	Sensors []SensorEntry `json:"sensors"`
}

// DeviceEntry describes a discovered device.
type DeviceEntry struct {
	DeviceSlug   string `json:"device_slug"`
	Model        string `json:"model"`
	Serial       string `json:"serial"`
	Name         string `json:"name"`
	Manufacturer string `json:"manufacturer,omitempty"`
}

// SensorEntry describes a single sensor reading with its HA config metadata.
type SensorEntry struct {
	DeviceSlug  string `json:"device_slug"`
	SensorSlug  string `json:"sensor_slug"`
	SensorName  string `json:"sensor_name"`
	Value       string `json:"value"`
	Unit        string `json:"unit"`
	DeviceClass string `json:"device_class,omitempty"`
	StateClass  string `json:"state_class,omitempty"`
	IsNumeric   bool   `json:"is_numeric"`
}

// Sort sorts devices by slug and sensors by device_slug then sensor_slug
// for stable, diffable output.
func (s *Snapshot) Sort() {
	sort.Slice(s.Devices, func(i, j int) bool {
		return s.Devices[i].DeviceSlug < s.Devices[j].DeviceSlug
	})
	sort.Slice(s.Sensors, func(i, j int) bool {
		if s.Sensors[i].DeviceSlug != s.Sensors[j].DeviceSlug {
			return s.Sensors[i].DeviceSlug < s.Sensors[j].DeviceSlug
		}
		return s.Sensors[i].SensorSlug < s.Sensors[j].SensorSlug
	})
}

// JSON returns the snapshot as indented JSON.
func (s *Snapshot) JSON() ([]byte, error) {
	s.Sort()
	return json.MarshalIndent(s, "", "  ")
}
