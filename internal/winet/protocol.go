package winet

import (
	"encoding/json"
	"fmt"
	"time"
)

// QueryStage represents a type of data query to execute against a device.
type QueryStage int

const (
	StageReal        QueryStage = iota // Realtime metrics
	StageDirect                        // MPPT string data
	StageRealBattery                   // Battery-specific metrics
)

// DeviceTypeStages maps WiNet device type codes to the query stages needed.
var DeviceTypeStages = map[int][]QueryStage{
	0:  {StageReal, StageDirect},                   // Inverter with MPPT
	8:  {StageReal},                                // Wireless Smart Meter
	11: {StageReal},                                // CT Box
	13: {StageReal},                                // PID Box
	14: {StageReal},                                // Combiner Box
	15: {StageReal},                                // Battery Inverter
	18: {StageReal},                                // Wireless Power Box
	20: {StageReal},                                // Wireless GPRS Box
	21: {StageReal, StageDirect},                   // String inverter with MPPT
	23: {StageReal},                                // Control Box
	24: {StageReal},                                // Environmental Box
	25: {StageReal},                                // DC Isolation Meter
	34: {StageReal},                                // Enhanced Ethernet Module
	35: {StageReal, StageRealBattery, StageDirect}, // Hybrid inverter with battery
	36: {StageReal},                                // Other
}

// NumericUnits lists units that indicate numeric sensor values.
var NumericUnits = map[string]bool{
	"A": true, "%": true, "kW": true, "kWh": true, "℃": true,
	"V": true, "kvar": true, "var": true, "Hz": true, "kVA": true,
	"kΩ": true, "W": true, "Wh": true,
}

// Service name constants used in WiNet protocol messages.
const (
	ServiceConnect     = "connect"
	ServiceLogin       = "login"
	ServiceDeviceList  = "devicelist"
	ServiceReal        = "real"
	ServiceRealBattery = "real_battery"
	ServiceDirect      = "direct"
	ServiceNotice      = "notice"
)

// WiNet firmware versions.
const (
	VersionWiNetS1    = 1 // WiNet-S (no ip field in connect response)
	VersionWiNetS2Old = 2 // WiNet-S2 older firmware (has ip, no forceModifyPasswd)
	VersionWiNetS2New = 3 // WiNet-S2 newer firmware (has forceModifyPasswd)
)

// --- Request messages ---

// All requests are sent as top-level JSON with lang, token, service, etc.

type ConnectRequest struct {
	Lang    string `json:"lang"`
	Token   string `json:"token"`
	Service string `json:"service"`
}

func NewConnectRequest(lang string) ConnectRequest {
	return ConnectRequest{
		Lang:    lang,
		Token:   "",
		Service: ServiceConnect,
	}
}

type LoginRequest struct {
	Lang     string `json:"lang"`
	Token    string `json:"token"`
	Service  string `json:"service"`
	Username string `json:"username"`
	Passwd   string `json:"passwd"`
}

func NewLoginRequest(lang, token, username, password string) LoginRequest {
	return LoginRequest{
		Lang:     lang,
		Token:    token,
		Service:  ServiceLogin,
		Username: username,
		Passwd:   password,
	}
}

type DeviceListRequest struct {
	Lang         string `json:"lang"`
	Token        string `json:"token"`
	Service      string `json:"service"`
	Type         string `json:"type"`
	IsCheckToken string `json:"is_check_token"`
}

func NewDeviceListRequest(lang, token string) DeviceListRequest {
	return DeviceListRequest{
		Lang:         lang,
		Token:        token,
		Service:      ServiceDeviceList,
		Type:         "0",
		IsCheckToken: "0",
	}
}

type DataRequest struct {
	Lang       string `json:"lang"`
	Token      string `json:"token"`
	Service    string `json:"service"`
	DevID      string `json:"dev_id"`
	Time123456 int64  `json:"time123456"`
}

func NewDataRequest(lang, token, service, devID string) DataRequest {
	return DataRequest{
		Lang:       lang,
		Token:      token,
		Service:    service,
		DevID:      devID,
		Time123456: time.Now().UnixMilli(),
	}
}

// --- Response messages ---
//
// All WiNet responses have the structure:
//
//	{
//	  "result_code": 1,
//	  "result_msg": "success",
//	  "result_data": {
//	    "service": "<service_name>",
//	    ...service-specific fields...
//	  }
//	}

// RawResponse is the outer envelope of every WiNet WebSocket message.
type RawResponse struct {
	ResultCode int             `json:"result_code"`
	ResultMsg  string          `json:"result_msg"`
	ResultData json.RawMessage `json:"result_data"`
}

// resultDataBase extracts just the service field from result_data.
type resultDataBase struct {
	Service string `json:"service"`
}

// ConnectData is the result_data payload for a "connect" response.
type ConnectData struct {
	Service           string `json:"service"`
	Token             string `json:"token"`
	UID               int    `json:"uid"`
	IP                string `json:"ip,omitempty"`
	ForceModifyPasswd *int   `json:"forceModifyPasswd,omitempty"`
}

// DetectVersion determines the WiNet firmware version from the connect response.
func (c *ConnectData) DetectVersion() int {
	if c.IP == "" {
		return VersionWiNetS1
	}
	if c.ForceModifyPasswd != nil {
		return VersionWiNetS2New
	}
	return VersionWiNetS2Old
}

// LoginData is the result_data payload for a "login" response.
type LoginData struct {
	Service string `json:"service"`
	Token   string `json:"token"`
	UID     int    `json:"uid"`
}

// Device represents a single device in the device list.
type Device struct {
	ID       int    `json:"id"`
	DevID    int    `json:"dev_id"`
	DevCode  int    `json:"dev_code"`
	DevType  int    `json:"dev_type"`
	DevSN    string `json:"dev_sn"`
	DevName  string `json:"dev_name"`
	DevModel string `json:"dev_model"`
	PortName string `json:"port_name"`
	PhysAddr string `json:"phys_addr"`
	LogcAddr string `json:"logc_addr"`
}

// DeviceListData is the result_data payload for a "devicelist" response.
type DeviceListData struct {
	Service string   `json:"service"`
	List    []Device `json:"list"`
	Count   int      `json:"count"`
}

// RealDataPoint is a single metric from a "real" or "real_battery" response.
type RealDataPoint struct {
	DataName  string `json:"data_name"`
	DataValue string `json:"data_value"`
	DataUnit  string `json:"data_unit"`
}

// RealData is the result_data payload for "real" and "real_battery" responses.
type RealData struct {
	Service string          `json:"service"`
	List    []RealDataPoint `json:"list"`
	Count   int             `json:"count"`
}

// DirectString is a single MPPT string from a "direct" response.
type DirectString struct {
	Name        string `json:"name"`
	Voltage     string `json:"voltage"`
	VoltageUnit string `json:"voltage_unit"`
	Current     string `json:"current"`
	CurrentUnit string `json:"current_unit"`
}

// DirectData is the result_data payload for a "direct" response.
type DirectData struct {
	Service string         `json:"service"`
	List    []DirectString `json:"list"`
	Count   int            `json:"count"`
}

// NoticeData is the result_data payload for a "notice" response.
type NoticeData struct {
	Service string `json:"service"`
}

// ParsedResponse holds a parsed WiNet response with its typed result_data.
type ParsedResponse struct {
	ResultCode int
	ResultMsg  string
	Service    string
	Data       interface{} // One of: *ConnectData, *LoginData, *DeviceListData, *RealData, *DirectData, *NoticeData
}

// ParseResponse parses a raw WebSocket message into a ParsedResponse.
func ParseResponse(raw []byte) (*ParsedResponse, error) {
	var envelope RawResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	var base resultDataBase
	if err := json.Unmarshal(envelope.ResultData, &base); err != nil {
		return nil, fmt.Errorf("unmarshal result_data base: %w", err)
	}

	resp := &ParsedResponse{
		ResultCode: envelope.ResultCode,
		ResultMsg:  envelope.ResultMsg,
		Service:    base.Service,
	}

	switch base.Service {
	case ServiceConnect:
		var d ConnectData
		if err := json.Unmarshal(envelope.ResultData, &d); err != nil {
			return nil, fmt.Errorf("unmarshal connect data: %w", err)
		}
		resp.Data = &d

	case ServiceLogin:
		var d LoginData
		if err := json.Unmarshal(envelope.ResultData, &d); err != nil {
			return nil, fmt.Errorf("unmarshal login data: %w", err)
		}
		resp.Data = &d

	case ServiceDeviceList:
		var d DeviceListData
		if err := json.Unmarshal(envelope.ResultData, &d); err != nil {
			return nil, fmt.Errorf("unmarshal devicelist data: %w", err)
		}
		resp.Data = &d

	case ServiceReal, ServiceRealBattery:
		var d RealData
		if err := json.Unmarshal(envelope.ResultData, &d); err != nil {
			return nil, fmt.Errorf("unmarshal real data: %w", err)
		}
		resp.Data = &d

	case ServiceDirect:
		var d DirectData
		if err := json.Unmarshal(envelope.ResultData, &d); err != nil {
			return nil, fmt.Errorf("unmarshal direct data: %w", err)
		}
		resp.Data = &d

	case ServiceNotice:
		var d NoticeData
		if err := json.Unmarshal(envelope.ResultData, &d); err != nil {
			return nil, fmt.Errorf("unmarshal notice data: %w", err)
		}
		resp.Data = &d

	default:
		resp.Data = nil
	}

	return resp, nil
}
