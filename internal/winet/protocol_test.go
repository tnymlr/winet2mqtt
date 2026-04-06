package winet

import (
	"testing"
)

func TestParseResponse_Connect(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "success",
		"result_data": {
			"service": "connect",
			"token": "abc123",
			"uid": 1,
			"ip": "192.168.1.100",
			"forceModifyPasswd": 0
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Service != "connect" {
		t.Errorf("expected service connect, got %s", resp.Service)
	}
	if resp.ResultCode != 1 {
		t.Errorf("expected result_code 1, got %d", resp.ResultCode)
	}
	data, ok := resp.Data.(*ConnectData)
	if !ok {
		t.Fatalf("expected *ConnectData, got %T", resp.Data)
	}
	if data.Token != "abc123" {
		t.Errorf("expected token abc123, got %s", data.Token)
	}
	if data.IP != "192.168.1.100" {
		t.Errorf("expected ip 192.168.1.100, got %s", data.IP)
	}
}

func TestConnectData_DetectVersion(t *testing.T) {
	tests := []struct {
		name string
		data ConnectData
		want int
	}{
		{
			name: "WiNet-S1 (no IP)",
			data: ConnectData{Token: "t", IP: ""},
			want: VersionWiNetS1,
		},
		{
			name: "WiNet-S2 older (IP, no forceModifyPasswd)",
			data: ConnectData{Token: "t", IP: "192.168.1.1"},
			want: VersionWiNetS2Old,
		},
		{
			name: "WiNet-S2 newer (IP + forceModifyPasswd)",
			data: ConnectData{Token: "t", IP: "192.168.1.1", ForceModifyPasswd: intPtr(0)},
			want: VersionWiNetS2New,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.data.DetectVersion()
			if got != tt.want {
				t.Errorf("DetectVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}

func intPtr(i int) *int { return &i }

func TestParseResponse_Login(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "success",
		"result_data": {
			"service": "login",
			"token": "authenticated_token",
			"uid": 42
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Service != "login" {
		t.Errorf("expected service login, got %s", resp.Service)
	}
	data, ok := resp.Data.(*LoginData)
	if !ok {
		t.Fatalf("expected *LoginData, got %T", resp.Data)
	}
	if data.Token != "authenticated_token" {
		t.Errorf("expected authenticated_token, got %s", data.Token)
	}
}

func TestParseResponse_LoginFailed(t *testing.T) {
	raw := `{
		"result_code": 115,
		"result_msg": "I18N_COMMON_USR_PASSWD_ERROR_TIMES",
		"result_data": {
			"service": "login",
			"token": "",
			"uid": 0
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ResultCode != 115 {
		t.Errorf("expected result_code 115, got %d", resp.ResultCode)
	}
	if resp.ResultMsg != "I18N_COMMON_USR_PASSWD_ERROR_TIMES" {
		t.Errorf("unexpected result_msg: %s", resp.ResultMsg)
	}
}

func TestParseResponse_AccountLocked(t *testing.T) {
	raw := `{
		"result_code": 114,
		"result_msg": "I18N_COMMON_USR_ACCOUNT_LOCK",
		"result_data": {
			"service": "login",
			"token": "",
			"uid": 0
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ResultCode != 114 {
		t.Errorf("expected result_code 114, got %d", resp.ResultCode)
	}
}

func TestParseResponse_DeviceList(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "success",
		"result_data": {
			"service": "devicelist",
			"list": [
				{
					"id": 1,
					"dev_id": 1,
					"dev_code": 3343,
					"dev_type": 35,
					"dev_sn": "A2582008920",
					"dev_name": "SH10RS",
					"dev_model": "SH10RS",
					"port_name": "COM1",
					"phys_addr": "1",
					"logc_addr": "1"
				}
			],
			"count": 1
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, ok := resp.Data.(*DeviceListData)
	if !ok {
		t.Fatalf("expected *DeviceListData, got %T", resp.Data)
	}
	if len(data.List) != 1 {
		t.Fatalf("expected 1 device, got %d", len(data.List))
	}
	dev := data.List[0]
	if dev.DevSN != "A2582008920" {
		t.Errorf("expected serial A2582008920, got %s", dev.DevSN)
	}
	if dev.DevModel != "SH10RS" {
		t.Errorf("expected model SH10RS, got %s", dev.DevModel)
	}
	if dev.DevType != 35 {
		t.Errorf("expected dev_type 35, got %d", dev.DevType)
	}
}

func TestParseResponse_Real(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "success",
		"result_data": {
			"service": "real",
			"list": [
				{"data_name": "I18N_COMMON_TOTAL_ACTIVE_POWER", "data_value": "3.5", "data_unit": "kW"},
				{"data_name": "I18N_COMMON_AC_VOLTAGE", "data_value": "239.5", "data_unit": "V"},
				{"data_name": "I18N_COMMON_RUNNING_STATUS", "data_value": "--", "data_unit": ""}
			],
			"count": 3
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, ok := resp.Data.(*RealData)
	if !ok {
		t.Fatalf("expected *RealData, got %T", resp.Data)
	}
	if len(data.List) != 3 {
		t.Fatalf("expected 3 data points, got %d", len(data.List))
	}
	if data.List[0].DataValue != "3.5" {
		t.Errorf("expected value 3.5, got %s", data.List[0].DataValue)
	}
	if data.List[0].DataUnit != "kW" {
		t.Errorf("expected unit kW, got %s", data.List[0].DataUnit)
	}
}

func TestParseResponse_RealBattery(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "success",
		"result_data": {
			"service": "real_battery",
			"list": [
				{"data_name": "I18N_COMMON_BATTERY_SOC", "data_value": "85", "data_unit": "%"}
			],
			"count": 1
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Service != "real_battery" {
		t.Errorf("expected service real_battery, got %s", resp.Service)
	}
	data, ok := resp.Data.(*RealData)
	if !ok {
		t.Fatalf("expected *RealData, got %T", resp.Data)
	}
	if data.List[0].DataValue != "85" {
		t.Errorf("expected value 85, got %s", data.List[0].DataValue)
	}
}

func TestParseResponse_Direct(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "success",
		"result_data": {
			"service": "direct",
			"list": [
				{
					"name": "I18N_COMMON_GROUP_BUNCH_TITLE_AND%@1",
					"voltage": "350.2",
					"voltage_unit": "V",
					"current": "5.1",
					"current_unit": "A"
				},
				{
					"name": "I18N_COMMON_GROUP_BUNCH_TITLE_AND%@2",
					"voltage": "340.8",
					"voltage_unit": "V",
					"current": "4.9",
					"current_unit": "A"
				}
			],
			"count": 2
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, ok := resp.Data.(*DirectData)
	if !ok {
		t.Fatalf("expected *DirectData, got %T", resp.Data)
	}
	if len(data.List) != 2 {
		t.Fatalf("expected 2 strings, got %d", len(data.List))
	}
	if data.List[0].Voltage != "350.2" {
		t.Errorf("expected voltage 350.2, got %s", data.List[0].Voltage)
	}
}

func TestParseResponse_Notice(t *testing.T) {
	raw := `{
		"result_code": 100,
		"result_msg": "timeout",
		"result_data": {
			"service": "notice"
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Service != "notice" {
		t.Errorf("expected service notice, got %s", resp.Service)
	}
	if resp.ResultCode != 100 {
		t.Errorf("expected result_code 100, got %d", resp.ResultCode)
	}
}

func TestParseResponse_InternalError(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "I18N_COMMON_INTER_ABNORMAL",
		"result_data": {
			"service": "real",
			"list": [],
			"count": 0
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ResultMsg != "I18N_COMMON_INTER_ABNORMAL" {
		t.Errorf("expected internal error msg, got %s", resp.ResultMsg)
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	_, err := ParseResponse([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseResponse_UnknownService(t *testing.T) {
	raw := `{
		"result_code": 1,
		"result_msg": "success",
		"result_data": {
			"service": "unknown_service"
		}
	}`

	resp, err := ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Service != "unknown_service" {
		t.Errorf("expected service unknown_service, got %s", resp.Service)
	}
	if resp.Data != nil {
		t.Errorf("expected nil data for unknown service, got %v", resp.Data)
	}
}

func TestNewConnectRequest(t *testing.T) {
	req := NewConnectRequest("en_US")
	if req.Service != "connect" {
		t.Errorf("expected service connect, got %s", req.Service)
	}
	if req.Lang != "en_US" {
		t.Errorf("expected lang en_US, got %s", req.Lang)
	}
	if req.Token != "" {
		t.Errorf("expected empty token, got %s", req.Token)
	}
}

func TestNewLoginRequest(t *testing.T) {
	req := NewLoginRequest("en_US", "tok", "admin", "pass")
	if req.Service != "login" {
		t.Errorf("expected service login, got %s", req.Service)
	}
	if req.Token != "tok" {
		t.Errorf("expected token tok, got %s", req.Token)
	}
	if req.Username != "admin" {
		t.Errorf("expected username admin, got %s", req.Username)
	}
	if req.Passwd != "pass" {
		t.Errorf("expected passwd pass, got %s", req.Passwd)
	}
}

func TestNewDeviceListRequest(t *testing.T) {
	req := NewDeviceListRequest("en_US", "tok")
	if req.Service != "devicelist" {
		t.Errorf("expected service devicelist, got %s", req.Service)
	}
	if req.Type != "0" {
		t.Errorf("expected type 0, got %s", req.Type)
	}
	if req.IsCheckToken != "0" {
		t.Errorf("expected is_check_token 0, got %s", req.IsCheckToken)
	}
}

func TestNewDataRequest(t *testing.T) {
	req := NewDataRequest("en_US", "tok", "real", "1")
	if req.Service != "real" {
		t.Errorf("expected service real, got %s", req.Service)
	}
	if req.DevID != "1" {
		t.Errorf("expected dev_id 1, got %s", req.DevID)
	}
	if req.Time123456 == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestDeviceTypeStages(t *testing.T) {
	// Inverter with MPPT: real + direct
	stages := DeviceTypeStages[0]
	if len(stages) != 2 || stages[0] != StageReal || stages[1] != StageDirect {
		t.Errorf("type 0: expected [StageReal, StageDirect], got %v", stages)
	}

	// Smart meter: real only
	stages = DeviceTypeStages[8]
	if len(stages) != 1 || stages[0] != StageReal {
		t.Errorf("type 8: expected [StageReal], got %v", stages)
	}

	// Hybrid inverter: real + real_battery + direct
	stages = DeviceTypeStages[35]
	if len(stages) != 3 {
		t.Errorf("type 35: expected 3 stages, got %d", len(stages))
	}
	if stages[0] != StageReal || stages[1] != StageRealBattery || stages[2] != StageDirect {
		t.Errorf("type 35: expected [StageReal, StageRealBattery, StageDirect], got %v", stages)
	}

	// Unknown type
	_, ok := DeviceTypeStages[999]
	if ok {
		t.Error("expected unknown type 999 to not be in map")
	}
}

func TestNumericUnits(t *testing.T) {
	numeric := []string{"A", "%", "kW", "kWh", "℃", "V", "kvar", "var", "Hz", "kVA", "kΩ", "W", "Wh"}
	for _, u := range numeric {
		if !NumericUnits[u] {
			t.Errorf("expected %q to be numeric", u)
		}
	}

	nonNumeric := []string{"", "lqi", "text", "foo"}
	for _, u := range nonNumeric {
		if NumericUnits[u] {
			t.Errorf("expected %q to not be numeric", u)
		}
	}
}
