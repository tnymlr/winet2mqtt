package winet

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// mockWiNet simulates a WiNet dongle WebSocket server.
type mockWiNet struct {
	t          *testing.T
	upgrader   websocket.Upgrader
	server     *httptest.Server
	mu         sync.Mutex
	devices    []Device
	realData   []RealDataPoint
	directData []DirectString
}

func newMockWiNet(t *testing.T) *mockWiNet {
	m := &mockWiNet{
		t: t,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
		devices: []Device{
			{
				ID: 1, DevID: 1, DevCode: 3343, DevType: 35,
				DevSN: "TEST123", DevName: "SH10RS", DevModel: "SH10RS",
				PortName: "COM1", PhysAddr: "1", LogcAddr: "1",
			},
		},
		realData: []RealDataPoint{
			{DataName: "I18N_COMMON_TOTAL_ACTIVE_POWER", DataValue: "3.5", DataUnit: "kW"},
			{DataName: "I18N_COMMON_AC_VOLTAGE", DataValue: "239.5", DataUnit: "V"},
			{DataName: "I18N_COMMON_RUNNING_STATUS", DataValue: "Running", DataUnit: ""},
		},
		directData: []DirectString{
			{
				Name:    "I18N_COMMON_GROUP_BUNCH_TITLE_AND%@1",
				Voltage: "350.2", VoltageUnit: "V",
				Current: "5.1", CurrentUnit: "A",
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/home/overview", m.handleWS)
	mux.HandleFunc("/i18n/en_US.properties", m.handleProperties)
	m.server = httptest.NewServer(mux)
	return m
}

func (m *mockWiNet) close() {
	m.server.Close()
}

func (m *mockWiNet) host() string {
	return strings.TrimPrefix(m.server.URL, "http://")
}

func (m *mockWiNet) handleProperties(w http.ResponseWriter, _ *http.Request) {
	props := `I18N_COMMON_TOTAL_ACTIVE_POWER=Total active power
I18N_COMMON_AC_VOLTAGE=AC voltage
I18N_COMMON_RUNNING_STATUS=Running status
I18N_COMMON_GROUP_BUNCH_TITLE_AND=String {0}
I18N_COMMON_BATTERY_SOC=Battery level (SOC)`
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(props))
}

func (m *mockWiNet) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		m.t.Logf("upgrade error: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var req map[string]interface{}
		if err := json.Unmarshal(msg, &req); err != nil {
			return
		}

		service, _ := req["service"].(string)
		var resp interface{}

		switch service {
		case ServiceConnect:
			resp = map[string]interface{}{
				"result_code": 1,
				"result_msg":  "success",
				"result_data": map[string]interface{}{
					"service":           ServiceConnect,
					"token":             "test_token",
					"uid":               1,
					"ip":                "192.168.1.100",
					"forceModifyPasswd": 0,
				},
			}
		case ServiceLogin:
			resp = map[string]interface{}{
				"result_code": 1,
				"result_msg":  "success",
				"result_data": map[string]interface{}{
					"service": ServiceLogin,
					"token":   "auth_token",
					"uid":     1,
				},
			}
		case ServiceDeviceList:
			m.mu.Lock()
			resp = map[string]interface{}{
				"result_code": 1,
				"result_msg":  "success",
				"result_data": map[string]interface{}{
					"service": ServiceDeviceList,
					"list":    m.devices,
					"count":   len(m.devices),
				},
			}
			m.mu.Unlock()
		case ServiceReal, ServiceRealBattery:
			m.mu.Lock()
			resp = map[string]interface{}{
				"result_code": 1,
				"result_msg":  "success",
				"result_data": map[string]interface{}{
					"service": service,
					"list":    m.realData,
					"count":   len(m.realData),
				},
			}
			m.mu.Unlock()
		case ServiceDirect:
			m.mu.Lock()
			resp = map[string]interface{}{
				"result_code": 1,
				"result_msg":  "success",
				"result_data": map[string]interface{}{
					"service": ServiceDirect,
					"list":    m.directData,
					"count":   len(m.directData),
				},
			}
			m.mu.Unlock()
		default:
			resp = map[string]interface{}{
				"result_code": 0,
				"result_msg":  "unknown service",
				"result_data": map[string]interface{}{
					"service": service,
				},
			}
		}

		data, _ := json.Marshal(resp)
		_ = conn.WriteMessage(websocket.TextMessage, data)
	}
}

func TestClient_FullPollCycle(t *testing.T) {
	mock := newMockWiNet(t)
	defer mock.close()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx := context.Background()
	props, _, err := FetchProperties(ctx, logger, mock.host(), "en_US", false)
	if err != nil {
		t.Fatalf("fetch properties: %v", err)
	}

	var result []DeviceData
	var mu sync.Mutex
	done := make(chan struct{})

	client := NewClient(
		mock.host(), "admin", "password",
		1, false, props, logger,
		func(devices []DeviceData) {
			mu.Lock()
			result = devices
			mu.Unlock()
			close(done)
		},
	)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	go func() {
		_ = client.Run(ctx)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout waiting for poll result")
	}
	cancel()
	client.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(result) != 1 {
		t.Fatalf("expected 1 device, got %d", len(result))
	}

	dev := result[0]
	if dev.Device.DevSN != "TEST123" {
		t.Errorf("expected serial TEST123, got %s", dev.Device.DevSN)
	}

	// Device type 35 = real + real_battery + direct.
	// real: 3 data points (1 skipped if "--", but we have none dashed) = 3 sensors
	// real_battery: same 3 data points again = 3 sensors
	// direct: 1 string × 3 (V,A,W) + 1 total = 4 sensors
	// Total: 3 + 3 + 4 = 10
	if len(dev.Sensors) != 10 {
		t.Errorf("expected 10 sensors, got %d", len(dev.Sensors))
		for i, s := range dev.Sensors {
			t.Logf("  sensor[%d]: %s = %s %s", i, s.Name, s.Value, s.Unit)
		}
	}

	// Verify some specific sensors.
	sensorMap := make(map[string]SensorData)
	for _, s := range dev.Sensors {
		sensorMap[s.Name] = s
	}

	if s, ok := sensorMap["Total active power"]; !ok {
		t.Error("missing 'Total active power' sensor")
	} else if s.Value != "3.5" || s.Unit != "kW" {
		t.Errorf("Total active power: got %s %s", s.Value, s.Unit)
	}

	if s, ok := sensorMap["String 1 Voltage"]; !ok {
		t.Error("missing 'String 1 Voltage' sensor")
	} else if s.Value != "350.2" || s.Unit != "V" {
		t.Errorf("String 1 Voltage: got %s %s", s.Value, s.Unit)
	}

	if s, ok := sensorMap["MPPT Total Power"]; !ok {
		t.Error("missing 'MPPT Total Power' sensor")
	} else if s.Value != "1786.02" || s.Unit != "W" {
		t.Errorf("MPPT Total Power: got %s %s", s.Value, s.Unit)
	}
}

func TestClient_AuthFailure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i18n/en_US.properties" {
			_, _ = w.Write([]byte("KEY=Value"))
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req map[string]interface{}
			_ = json.Unmarshal(msg, &req)
			service, _ := req["service"].(string)

			var resp interface{}
			switch service {
			case ServiceConnect:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{"service": ServiceConnect, "token": "t", "uid": 1},
				}
			case ServiceLogin:
				resp = map[string]interface{}{
					"result_code": 115, "result_msg": "I18N_COMMON_USR_PASSWD_ERROR_TIMES",
					"result_data": map[string]interface{}{"service": ServiceLogin, "token": "", "uid": 0},
				}
			}
			data, _ := json.Marshal(resp)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	props, _, _ := FetchProperties(ctx, logger, host, "en_US", false)
	client := NewClient(host, "admin", "wrong", 1, false, props, logger, nil)

	err := client.Run(ctx)
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected auth failure error, got: %v", err)
	}
}

func TestClient_AccountLocked(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i18n/en_US.properties" {
			_, _ = w.Write([]byte("KEY=Value"))
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req map[string]interface{}
			_ = json.Unmarshal(msg, &req)
			service, _ := req["service"].(string)

			var resp interface{}
			switch service {
			case ServiceConnect:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{"service": ServiceConnect, "token": "t", "uid": 1},
				}
			case ServiceLogin:
				resp = map[string]interface{}{
					"result_code": 114, "result_msg": "I18N_COMMON_USR_ACCOUNT_LOCK",
					"result_data": map[string]interface{}{"service": ServiceLogin, "token": "", "uid": 0},
				}
			}
			data, _ := json.Marshal(resp)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	props, _, _ := FetchProperties(ctx, logger, host, "en_US", false)
	client := NewClient(host, "admin", "pass", 1, false, props, logger, nil)

	err := client.Run(ctx)
	if err == nil {
		t.Fatal("expected account locked error")
	}
	if !strings.Contains(err.Error(), "account locked") {
		t.Errorf("expected account locked error, got: %v", err)
	}
}

func TestClient_NoticeResponse(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	queryCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i18n/en_US.properties" {
			_, _ = w.Write([]byte("KEY=Value"))
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req map[string]interface{}
			_ = json.Unmarshal(msg, &req)
			service, _ := req["service"].(string)

			var resp interface{}
			switch service {
			case ServiceConnect:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{"service": ServiceConnect, "token": "t", "uid": 1},
				}
			case ServiceLogin:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{"service": ServiceLogin, "token": "auth", "uid": 1},
				}
			case ServiceDeviceList:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{
						"service": ServiceDeviceList,
						"list":    []Device{{DevID: 1, DevType: 8, DevSN: "T1", DevModel: "M1"}},
						"count":   1,
					},
				}
			case ServiceReal:
				queryCount++
				// Respond with a notice instead of real data.
				resp = map[string]interface{}{
					"result_code": 100, "result_msg": "timeout",
					"result_data": map[string]interface{}{"service": ServiceNotice},
				}
			}
			data, _ := json.Marshal(resp)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	props, _, _ := FetchProperties(ctx, logger, host, "en_US", false)
	client := NewClient(host, "admin", "pass", 1, false, props, logger, nil)

	// Run should reconnect on notice, so it won't return quickly.
	// Just let it run for a bit and verify it handled the notice.
	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()
	_ = client.Run(ctx)

	if queryCount == 0 {
		t.Error("expected at least one real query to be sent")
	}
}

func TestClient_InternalError(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/i18n/en_US.properties" {
			_, _ = w.Write([]byte("KEY=Value"))
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req map[string]interface{}
			_ = json.Unmarshal(msg, &req)
			service, _ := req["service"].(string)

			var resp interface{}
			switch service {
			case ServiceConnect:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{"service": ServiceConnect, "token": "t", "uid": 1},
				}
			case ServiceLogin:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{"service": ServiceLogin, "token": "auth", "uid": 1},
				}
			case ServiceDeviceList:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "success",
					"result_data": map[string]interface{}{
						"service": ServiceDeviceList,
						"list":    []Device{{DevID: 1, DevType: 8, DevSN: "T1", DevModel: "M1"}},
						"count":   1,
					},
				}
			case ServiceReal:
				resp = map[string]interface{}{
					"result_code": 1, "result_msg": "I18N_COMMON_INTER_ABNORMAL",
					"result_data": map[string]interface{}{
						"service": ServiceReal,
						"list":    []interface{}{},
						"count":   0,
					},
				}
			}
			data, _ := json.Marshal(resp)
			_ = conn.WriteMessage(websocket.TextMessage, data)
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	props, _, _ := FetchProperties(ctx, logger, host, "en_US", false)
	called := false
	client := NewClient(host, "admin", "pass", 1, false, props, logger, func(_ []DeviceData) {
		called = true
	})

	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()
	_ = client.Run(ctx)

	// Should not have called the callback since the device returned an internal error.
	if called {
		t.Error("callback should not have been called on internal error")
	}
}

func TestClient_UnknownDeviceType(t *testing.T) {
	mock := newMockWiNet(t)
	// Replace devices with an unknown type.
	mock.mu.Lock()
	mock.devices = []Device{
		{DevID: 1, DevType: 999, DevSN: "UNKNOWN", DevModel: "X"},
	}
	mock.mu.Unlock()
	defer mock.close()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	props, _, _ := FetchProperties(ctx, logger, mock.host(), "en_US", false)

	var result []DeviceData
	done := make(chan struct{})
	client := NewClient(mock.host(), "admin", "pass", 1, false, props, logger, func(devices []DeviceData) {
		result = devices
		close(done)
	})

	go func() { _ = client.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timeout")
	}
	cancel()
	client.Close()

	// Unknown device types are skipped, so callback gets empty data.
	if len(result) != 0 {
		t.Errorf("expected 0 devices for unknown type, got %d", len(result))
	}
}

func TestFetchProperties(t *testing.T) {
	mock := newMockWiNet(t)
	defer mock.close()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	props, ssl, err := FetchProperties(context.Background(), logger, mock.host(), "en_US", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ssl {
		t.Error("expected ssl=false for plain HTTP test server")
	}
	if len(props) == 0 {
		t.Error("expected non-empty properties")
	}
	if props["I18N_COMMON_AC_VOLTAGE"] != "AC voltage" {
		t.Errorf("expected 'AC voltage', got %q", props["I18N_COMMON_AC_VOLTAGE"])
	}
}
