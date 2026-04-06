package mqtt

import (
	"testing"
)

func TestMakeSensorSlug(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"AC voltage", "ac_voltage"},
		{"Total active power", "total_active_power"},
		{"Battery level (SOC)", "battery_level_soc"},
		{"Max. charging current (BMS)", "max_charging_current_bms"},
		{"MPPT1 Voltage", "mppt1_voltage"},
		{"MPPT Total Power", "mppt_total_power"},
		{"String 1 Voltage", "string_1_voltage"},
		{"Daily PV yield", "daily_pv_yield"},
		{"Total power factor", "total_power_factor"},
		{"Phase A backup voltage", "phase_a_backup_voltage"},
		{"Feed-in energy today (PV)", "feed_in_energy_today_pv"},
		{"Daily self-consumption rate", "daily_self_consumption_rate"},
		{"Meter Phase-A current", "meter_phase_a_current"},
		{"Total feed-in energy", "total_feed_in_energy"},
		{"Running status", "running_status"},
		{"Battery SOH", "battery_soh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MakeSensorSlug(tt.name)
			if got != tt.want {
				t.Errorf("MakeSensorSlug(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestNormalizeUnit(t *testing.T) {
	tests := []struct {
		unit     string
		wantUnit string
		wantMult float64
	}{
		{"kW", "kW", 1},
		{"kWp", "kW", 1},
		{"℃", "°C", 1},
		{"kvar", "var", 1000},
		{"kVA", "VA", 1000},
		{"V", "V", 1},
		{"A", "A", 1},
		{"W", "W", 1},
		{"Hz", "Hz", 1},
		{"%", "%", 1},
		{"", "", 1},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			gotUnit, gotMult := NormalizeUnit(tt.unit)
			if gotUnit != tt.wantUnit {
				t.Errorf("NormalizeUnit(%q) unit = %q, want %q", tt.unit, gotUnit, tt.wantUnit)
			}
			if gotMult != tt.wantMult {
				t.Errorf("NormalizeUnit(%q) multiplier = %f, want %f", tt.unit, gotMult, tt.wantMult)
			}
		})
	}
}
