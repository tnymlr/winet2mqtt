package winet

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// SensorData represents a single sensor reading from a device.
type SensorData struct {
	Name  string
	Value string
	Unit  string
}

// DeviceData holds all sensor readings for a single device.
type DeviceData struct {
	Device  Device
	Sensors []SensorData
}

// UpdateCallback is called when a full poll cycle completes for all devices.
type UpdateCallback func(devices []DeviceData)

// Client connects to a WiNet dongle via WebSocket and polls device data.
type Client struct {
	host     string
	username string
	password string
	lang     string
	ssl      bool
	pollSec  int

	props    Properties
	logger   *slog.Logger
	callback UpdateCallback

	mu         sync.Mutex
	conn       *websocket.Conn
	token      string
	version    int
	devices    []Device
	deviceData map[int]*deviceState // keyed by dev_id
	cancelFunc context.CancelFunc
}

type deviceState struct {
	device  Device
	stages  []QueryStage
	sensors []SensorData
}

// NewClient creates a new WiNet WebSocket client.
func NewClient(host, username, password string, pollSec int, ssl bool, props Properties, logger *slog.Logger, cb UpdateCallback) *Client {
	return &Client{
		host:       host,
		username:   username,
		password:   password,
		lang:       "en_US",
		ssl:        ssl,
		pollSec:    pollSec,
		props:      props,
		logger:     logger,
		callback:   cb,
		deviceData: make(map[int]*deviceState),
	}
}

// ErrAccountLocked is returned when the WiNet account is locked.
var ErrAccountLocked = fmt.Errorf("account locked")

// ErrAuthFailed is returned when login credentials are rejected.
var ErrAuthFailed = fmt.Errorf("authentication failed")

// Run connects and runs the polling loop until the context is cancelled.
// It reconnects automatically on transient failures but returns immediately
// on authentication errors.
func (c *Client) Run(ctx context.Context) error {
	for {
		err := c.runOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Don't retry on auth failures — they won't resolve by reconnecting.
		if errors.Is(err, ErrAccountLocked) || errors.Is(err, ErrAuthFailed) {
			return err
		}
		c.logger.Warn("connection lost, reconnecting", "error", err)
		delay := time.Duration(c.pollSec*3) * time.Second
		if delay < 10*time.Second {
			delay = 10 * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.mu.Lock()
	c.cancelFunc = cancel
	c.mu.Unlock()

	conn, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	defer func() { _ = conn.Close() }()

	// Connect handshake.
	if err := c.sendJSON(NewConnectRequest(c.lang)); err != nil {
		return fmt.Errorf("send connect: %w", err)
	}

	resp, err := c.readResponse()
	if err != nil {
		return fmt.Errorf("read connect response: %w", err)
	}
	if resp.Service != ServiceConnect {
		return fmt.Errorf("expected connect response, got %s", resp.Service)
	}
	connData := resp.Data.(*ConnectData)
	c.token = connData.Token
	c.version = connData.DetectVersion()
	c.logger.Info("connected to WiNet", "version", c.version)

	// Login.
	if err := c.sendJSON(NewLoginRequest(c.lang, c.token, c.username, c.password)); err != nil {
		return fmt.Errorf("send login: %w", err)
	}

	resp, err = c.readResponse()
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}
	if resp.Service != ServiceLogin {
		return fmt.Errorf("expected login response, got %s", resp.Service)
	}
	if resp.ResultCode != 1 {
		if resp.ResultCode == 114 {
			return fmt.Errorf("login failed (code=%d msg=%s): %w", resp.ResultCode, resp.ResultMsg, ErrAccountLocked)
		}
		return fmt.Errorf("login failed (code=%d msg=%s): %w", resp.ResultCode, resp.ResultMsg, ErrAuthFailed)
	}
	loginData := resp.Data.(*LoginData)
	c.token = loginData.Token
	c.logger.Info("authenticated")

	// Get device list.
	if err := c.sendJSON(NewDeviceListRequest(c.lang, c.token)); err != nil {
		return fmt.Errorf("send devicelist: %w", err)
	}

	resp, err = c.readResponse()
	if err != nil {
		return fmt.Errorf("read devicelist response: %w", err)
	}
	if resp.Service != ServiceDeviceList {
		return fmt.Errorf("expected devicelist response, got %s", resp.Service)
	}
	devData := resp.Data.(*DeviceListData)
	c.devices = devData.List
	c.logger.Info("discovered devices", "count", len(c.devices))

	// Initialize device states.
	c.deviceData = make(map[int]*deviceState)
	for _, d := range c.devices {
		stages, ok := DeviceTypeStages[d.DevType]
		if !ok {
			c.logger.Warn("unknown device type, skipping", "dev_id", d.DevID, "dev_type", d.DevType)
			continue
		}
		c.deviceData[d.DevID] = &deviceState{
			device: d,
			stages: stages,
		}
	}

	return c.pollLoop(ctx)
}

func (c *Client) pollLoop(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(c.pollSec) * time.Second)
	defer ticker.Stop()

	// Do an immediate first poll.
	if err := c.pollAll(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.pollAll(ctx); err != nil {
				return err
			}
		}
	}
}

func (c *Client) pollAll(ctx context.Context) error {
	allData := make([]DeviceData, 0, len(c.deviceData))

	for _, ds := range c.deviceData {
		ds.sensors = nil

		for _, stage := range ds.stages {
			if err := ctx.Err(); err != nil {
				return err
			}
			sensors, err := c.queryStage(ds.device, stage)
			if err != nil {
				c.logger.Error("query stage failed", "dev_id", ds.device.DevID, "stage", stage, "error", err)
				return err
			}
			ds.sensors = append(ds.sensors, sensors...)
		}

		allData = append(allData, DeviceData{
			Device:  ds.device,
			Sensors: ds.sensors,
		})
	}

	if c.callback != nil {
		c.callback(allData)
	}
	return nil
}

func (c *Client) queryStage(dev Device, stage QueryStage) ([]SensorData, error) {
	var service string
	switch stage {
	case StageReal:
		service = ServiceReal
	case StageDirect:
		service = ServiceDirect
	case StageRealBattery:
		service = ServiceRealBattery
	}

	devID := strconv.Itoa(dev.DevID)
	if err := c.sendJSON(NewDataRequest(c.lang, c.token, service, devID)); err != nil {
		return nil, fmt.Errorf("send %s request: %w", service, err)
	}

	resp, err := c.readResponse()
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", service, err)
	}

	// Check for internal error.
	if resp.ResultMsg == "I18N_COMMON_INTER_ABNORMAL" {
		return nil, fmt.Errorf("internal device error")
	}

	// Handle notice/error responses.
	if resp.Service == ServiceNotice {
		return nil, fmt.Errorf("server notice: code=%d msg=%s", resp.ResultCode, resp.ResultMsg)
	}

	switch resp.Service {
	case ServiceReal, ServiceRealBattery:
		realData := resp.Data.(*RealData)
		return c.parseRealData(realData), nil

	case ServiceDirect:
		directData := resp.Data.(*DirectData)
		return c.parseDirectData(directData), nil

	default:
		return nil, fmt.Errorf("unexpected response service: %s", resp.Service)
	}
}

func (c *Client) parseRealData(data *RealData) []SensorData {
	sensors := make([]SensorData, 0, len(data.List))
	for _, dp := range data.List {
		if dp.DataValue == "--" {
			continue
		}
		name := c.props.Resolve(dp.DataName)
		sensors = append(sensors, SensorData{
			Name:  name,
			Value: dp.DataValue,
			Unit:  dp.DataUnit,
		})
	}
	return sensors
}

func (c *Client) parseDirectData(data *DirectData) []SensorData {
	sensors := make([]SensorData, 0, len(data.List)*3+1)
	var totalPower float64

	for _, s := range data.List {
		// The name field is like "I18N_COMMON_GROUP_BUNCH_TITLE_AND%@1".
		// Split on '%' to separate the i18n key from the template parameter.
		parts := strings.SplitN(s.Name, "%", 2)
		name := c.props.Resolve(parts[0])
		if len(parts) > 1 {
			// Replace {0} with the index (strip leading '@').
			idx := strings.TrimPrefix(parts[1], "@")
			name = strings.ReplaceAll(name, "{0}", idx)
		}

		if s.Voltage == "--" && s.Current == "--" {
			continue
		}

		voltage, _ := strconv.ParseFloat(s.Voltage, 64)
		current, _ := strconv.ParseFloat(s.Current, 64)
		power := voltage * current

		sensors = append(sensors, SensorData{
			Name:  name + " Voltage",
			Value: s.Voltage,
			Unit:  s.VoltageUnit,
		})
		sensors = append(sensors, SensorData{
			Name:  name + " Current",
			Value: s.Current,
			Unit:  s.CurrentUnit,
		})
		sensors = append(sensors, SensorData{
			Name:  name + " Power",
			Value: strconv.FormatFloat(math.Round(power*100)/100, 'f', -1, 64),
			Unit:  "W",
		})

		totalPower += power
	}

	if len(sensors) > 0 {
		sensors = append(sensors, SensorData{
			Name:  "MPPT Total Power",
			Value: strconv.FormatFloat(math.Round(totalPower*100)/100, 'f', -1, 64),
			Unit:  "W",
		})
	}

	return sensors
}

func (c *Client) dial(ctx context.Context) (*websocket.Conn, error) {
	var url string
	host := c.host
	hasPort := strings.Contains(host, ":")
	if c.ssl {
		if hasPort {
			url = fmt.Sprintf("wss://%s/ws/home/overview", host)
		} else {
			url = fmt.Sprintf("wss://%s:443/ws/home/overview", host)
		}
	} else {
		if hasPort {
			url = fmt.Sprintf("ws://%s/ws/home/overview", host)
		} else {
			url = fmt.Sprintf("ws://%s:8082/ws/home/overview", host)
		}
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	if c.ssl {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // self-signed certs on WiNet dongles
	}

	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial %s: %w", url, err)
	}

	return conn, nil
}

func (c *Client) sendJSON(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	c.logger.Debug("sending", "message", string(data))
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *Client) readResponse() (*ParsedResponse, error) {
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}
	c.logger.Debug("received", "message", string(data))
	return ParseResponse(data)
}

// Close shuts down the client.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	if c.conn != nil {
		_ = c.conn.Close()
	}
}
