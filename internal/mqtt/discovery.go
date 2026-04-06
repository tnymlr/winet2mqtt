package mqtt

// Home Assistant MQTT discovery device classes and state classes.
// See: https://www.home-assistant.io/integrations/sensor.mqtt/

// DeviceClasses maps unit strings to HA device_class values.
var DeviceClasses = map[string]string{
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

// StateClasses maps unit strings to HA state_class values.
var StateClasses = map[string]string{
	"W":    "measurement",
	"kW":   "measurement",
	"V":    "measurement",
	"A":    "measurement",
	"℃":    "measurement",
	"°C":   "measurement",
	"Hz":   "measurement",
	"%":    "measurement",
	"var":  "measurement",
	"kvar": "measurement",
	"kΩ":   "measurement",
	"VA":   "measurement",
	"kVA":  "measurement",
}

// NormalizeUnit converts WiNet units to HA-compatible units.
func NormalizeUnit(unit string) (string, float64) {
	switch unit {
	case "kWp":
		return "kW", 1
	case "℃":
		return "°C", 1
	case "kvar":
		return "var", 1000
	case "kVA":
		return "VA", 1000
	default:
		return unit, 1
	}
}

// ConfigPayload is the HA MQTT discovery config message.
type ConfigPayload struct {
	Name              string       `json:"name"`
	StateTopic        string       `json:"state_topic"`
	UniqueID          string       `json:"unique_id"`
	ValueTemplate     string       `json:"value_template"`
	Device            DeviceConfig `json:"device"`
	Encoding          string       `json:"encoding,omitempty"`
	UnitOfMeasurement string       `json:"unit_of_measurement,omitempty"`
	StateClass        string       `json:"state_class,omitempty"`
	DeviceClass       string       `json:"device_class,omitempty"`
}

// DeviceConfig is the HA device block in discovery messages.
type DeviceConfig struct {
	Name         string   `json:"name"`
	Identifiers  []string `json:"identifiers"`
	Model        string   `json:"model"`
	Manufacturer string   `json:"manufacturer,omitempty"`
}

// DeviceRegistration is the top-level device config message.
type DeviceRegistration struct {
	Abbreviation string       `json:"~"`
	Name         string       `json:"name"`
	UniqueID     string       `json:"unique_id"`
	StateTopic   string       `json:"state_topic"`
	Device       DeviceConfig `json:"device"`
}
