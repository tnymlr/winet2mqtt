package winet

import (
	"testing"
)

func TestParseRealData(t *testing.T) {
	props := Properties{
		"I18N_COMMON_TOTAL_ACTIVE_POWER": "Total active power",
		"I18N_COMMON_AC_VOLTAGE":         "AC voltage",
		"I18N_COMMON_RUNNING_STATUS":     "Running status",
	}

	c := &Client{props: props}

	data := &RealData{
		List: []RealDataPoint{
			{DataName: "I18N_COMMON_TOTAL_ACTIVE_POWER", DataValue: "3.5", DataUnit: "kW"},
			{DataName: "I18N_COMMON_AC_VOLTAGE", DataValue: "239.5", DataUnit: "V"},
			{DataName: "I18N_COMMON_RUNNING_STATUS", DataValue: "--", DataUnit: ""},
			{DataName: "I18N_UNKNOWN_KEY", DataValue: "42", DataUnit: "W"},
		},
	}

	sensors := c.parseRealData(data)

	// "--" values should be skipped.
	if len(sensors) != 3 {
		t.Fatalf("expected 3 sensors, got %d", len(sensors))
	}

	if sensors[0].Name != "Total active power" {
		t.Errorf("expected 'Total active power', got %q", sensors[0].Name)
	}
	if sensors[0].Value != "3.5" {
		t.Errorf("expected value 3.5, got %s", sensors[0].Value)
	}
	if sensors[0].Unit != "kW" {
		t.Errorf("expected unit kW, got %s", sensors[0].Unit)
	}

	// Unknown i18n key should pass through as-is.
	if sensors[2].Name != "I18N_UNKNOWN_KEY" {
		t.Errorf("expected raw key passthrough, got %q", sensors[2].Name)
	}
}

func TestParseDirectData(t *testing.T) {
	props := Properties{
		"I18N_COMMON_GROUP_BUNCH_TITLE_AND": "String {0}",
	}

	c := &Client{props: props}

	data := &DirectData{
		List: []DirectString{
			{
				Name:        "I18N_COMMON_GROUP_BUNCH_TITLE_AND%@1",
				Voltage:     "350.2",
				VoltageUnit: "V",
				Current:     "5.1",
				CurrentUnit: "A",
			},
			{
				Name:        "I18N_COMMON_GROUP_BUNCH_TITLE_AND%@2",
				Voltage:     "340.8",
				VoltageUnit: "V",
				Current:     "4.9",
				CurrentUnit: "A",
			},
		},
	}

	sensors := c.parseDirectData(data)

	// 2 strings × 3 (voltage, current, power) + 1 total = 7
	if len(sensors) != 7 {
		t.Fatalf("expected 7 sensors, got %d", len(sensors))
	}

	// Check name resolution: "String {0}" with @1 → "String 1"
	if sensors[0].Name != "String 1 Voltage" {
		t.Errorf("expected 'String 1 Voltage', got %q", sensors[0].Name)
	}
	if sensors[0].Value != "350.2" {
		t.Errorf("expected voltage 350.2, got %s", sensors[0].Value)
	}

	if sensors[1].Name != "String 1 Current" {
		t.Errorf("expected 'String 1 Current', got %q", sensors[1].Name)
	}

	if sensors[2].Name != "String 1 Power" {
		t.Errorf("expected 'String 1 Power', got %q", sensors[2].Name)
	}
	// Power = 350.2 * 5.1 = 1786.02
	if sensors[2].Value != "1786.02" {
		t.Errorf("expected power 1786.02, got %s", sensors[2].Value)
	}
	if sensors[2].Unit != "W" {
		t.Errorf("expected unit W, got %s", sensors[2].Unit)
	}

	if sensors[3].Name != "String 2 Voltage" {
		t.Errorf("expected 'String 2 Voltage', got %q", sensors[3].Name)
	}

	// Last sensor should be MPPT Total Power.
	total := sensors[6]
	if total.Name != "MPPT Total Power" {
		t.Errorf("expected 'MPPT Total Power', got %q", total.Name)
	}
	// Total = 350.2*5.1 + 340.8*4.9 = 1786.02 + 1669.92 = 3455.94
	if total.Value != "3455.94" {
		t.Errorf("expected total 3455.94, got %s", total.Value)
	}
}

func TestParseDirectData_SkipsDashes(t *testing.T) {
	props := Properties{
		"I18N_COMMON_GROUP_BUNCH_TITLE_AND": "String {0}",
	}
	c := &Client{props: props}

	data := &DirectData{
		List: []DirectString{
			{
				Name:        "I18N_COMMON_GROUP_BUNCH_TITLE_AND%@1",
				Voltage:     "--",
				VoltageUnit: "V",
				Current:     "--",
				CurrentUnit: "A",
			},
		},
	}

	sensors := c.parseDirectData(data)
	if len(sensors) != 0 {
		t.Errorf("expected 0 sensors for dashed values, got %d", len(sensors))
	}
}

func TestParseDirectData_EmptyList(t *testing.T) {
	c := &Client{props: Properties{}}

	data := &DirectData{List: []DirectString{}}
	sensors := c.parseDirectData(data)

	if len(sensors) != 0 {
		t.Errorf("expected 0 sensors for empty list, got %d", len(sensors))
	}
}

func TestParseDirectData_UnresolvedKey(t *testing.T) {
	// Key not in properties — should pass through as-is, with %@ handling.
	c := &Client{props: Properties{}}

	data := &DirectData{
		List: []DirectString{
			{
				Name:        "UNKNOWN_KEY%@3",
				Voltage:     "100",
				VoltageUnit: "V",
				Current:     "2",
				CurrentUnit: "A",
			},
		},
	}

	sensors := c.parseDirectData(data)
	// 3 per string + 1 total = 4
	if len(sensors) != 4 {
		t.Fatalf("expected 4 sensors, got %d", len(sensors))
	}
	// Unresolved key should keep the raw key name.
	if sensors[0].Name != "UNKNOWN_KEY Voltage" {
		t.Errorf("expected 'UNKNOWN_KEY Voltage', got %q", sensors[0].Name)
	}
}
