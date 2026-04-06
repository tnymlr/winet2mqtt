package config

import (
	"testing"

	"github.com/spf13/viper"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				WiNetHost:    "192.168.1.100",
				MQTTUrl:      "tcp://localhost:1883",
				PollInterval: 10,
			},
			wantErr: false,
		},
		{
			name: "missing winet host",
			cfg: Config{
				MQTTUrl:      "tcp://localhost:1883",
				PollInterval: 10,
			},
			wantErr: true,
		},
		{
			name: "missing mqtt url",
			cfg: Config{
				WiNetHost:    "192.168.1.100",
				PollInterval: 10,
			},
			wantErr: true,
		},
		{
			name: "poll interval too low",
			cfg: Config{
				WiNetHost:    "192.168.1.100",
				MQTTUrl:      "tcp://localhost:1883",
				PollInterval: 0,
			},
			wantErr: true,
		},
		{
			name: "poll interval too high",
			cfg: Config{
				WiNetHost:    "192.168.1.100",
				MQTTUrl:      "tcp://localhost:1883",
				PollInterval: 3601,
			},
			wantErr: true,
		},
		{
			name: "poll interval at boundary low",
			cfg: Config{
				WiNetHost:    "192.168.1.100",
				MQTTUrl:      "tcp://localhost:1883",
				PollInterval: 1,
			},
			wantErr: false,
		},
		{
			name: "poll interval at boundary high",
			cfg: Config{
				WiNetHost:    "192.168.1.100",
				MQTTUrl:      "tcp://localhost:1883",
				PollInterval: 3600,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSetDefaults(t *testing.T) {
	v := viper.New()
	viper.Reset()
	SetDefaults()

	if v := viper.GetString("mqtt-prefix"); v != "homeassistant" {
		t.Errorf("expected default mqtt-prefix homeassistant, got %s", v)
	}
	if v := viper.GetString("winet-username"); v != "admin" {
		t.Errorf("expected default winet-username admin, got %s", v)
	}
	if v := viper.GetString("winet-password"); v != "pw8888" {
		t.Errorf("expected default winet-password pw8888, got %s", v)
	}
	if v := viper.GetInt("poll-interval"); v != 10 {
		t.Errorf("expected default poll-interval 10, got %d", v)
	}
	if v := viper.GetInt("health-port"); v != 8081 {
		t.Errorf("expected default health-port 8081, got %d", v)
	}
	_ = v // suppress unused
}

func TestBindEnv(t *testing.T) {
	viper.Reset()
	SetDefaults()
	BindEnv()

	t.Setenv("WINET2MQTT_WINET_HOST", "10.0.0.1")
	t.Setenv("WINET2MQTT_MQTT_URL", "tcp://broker:1883")

	if v := viper.GetString("winet-host"); v != "10.0.0.1" {
		t.Errorf("expected winet-host from env, got %s", v)
	}
	if v := viper.GetString("mqtt-url"); v != "tcp://broker:1883" {
		t.Errorf("expected mqtt-url from env, got %s", v)
	}
}
