package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"winet2mqtt/internal/mqtt"
	"winet2mqtt/internal/snapshot"
	"winet2mqtt/internal/winet"
)

var pollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Run one poll cycle against WiNet and dump results as JSON",
	Long:  "Connects to the WiNet dongle, authenticates, discovers devices, runs a single poll cycle, and outputs all sensor data as a JSON snapshot for comparison.",
	RunE:  runPoll,
}

func init() {
	f := pollCmd.Flags()
	f.String("winet-host", "", "WiNet dongle IP or hostname (required)")
	f.String("winet-username", "admin", "WiNet dongle username")
	f.String("winet-password", "pw8888", "WiNet dongle password")
	f.StringP("output", "o", "", "Output file (default: stdout)")

	rootCmd.AddCommand(pollCmd)
}

func runPoll(cmd *cobra.Command, _ []string) error {
	bindFlags(cmd)

	winetHost, _ := cmd.Flags().GetString("winet-host")
	winetUser, _ := cmd.Flags().GetString("winet-username")
	winetPass, _ := cmd.Flags().GetString("winet-password")
	output, _ := cmd.Flags().GetString("output")

	if winetHost == "" {
		return fmt.Errorf("winet-host is required")
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Fetch properties.
	fmt.Fprintln(os.Stderr, "Fetching i18n properties...")
	props, ssl, err := winet.FetchProperties(ctx, logger, winetHost, "en_US", false)
	if err != nil {
		return fmt.Errorf("fetch properties: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Fetched %d properties (ssl=%v)\n", len(props), ssl)

	// Channel to receive one poll cycle.
	resultCh := make(chan []winet.DeviceData, 1)

	client := winet.NewClient(
		winetHost, winetUser, winetPass,
		1, // poll interval doesn't matter, we stop after one cycle
		ssl, props, logger,
		func(devices []winet.DeviceData) {
			resultCh <- devices
			cancel() // Stop after first cycle.
		},
	)

	// Run client in background.
	go func() {
		if err := client.Run(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "client error: %v\n", err)
			cancel()
		}
	}()

	// Wait for result or cancellation.
	var devices []winet.DeviceData
	select {
	case devices = <-resultCh:
		fmt.Fprintf(os.Stderr, "Poll complete: %d devices\n", len(devices))
	case <-ctx.Done():
		client.Close()
		return fmt.Errorf("cancelled before poll completed")
	}

	client.Close()

	// Build snapshot.
	snap := &snapshot.Snapshot{}

	for _, dev := range devices {
		deviceSlug := dev.Device.DevModel + "_" + dev.Device.DevSN

		snap.Devices = append(snap.Devices, snapshot.DeviceEntry{
			DeviceSlug:   deviceSlug,
			Model:        dev.Device.DevModel,
			Serial:       dev.Device.DevSN,
			Name:         dev.Device.DevModel + " " + dev.Device.DevSN,
			Manufacturer: "Sungrow",
		})

		for _, sensor := range dev.Sensors {
			sensorSlug := mqtt.MakeSensorSlug(sensor.Name)
			_, isNumeric := parseNumeric(sensor.Value, sensor.Unit)

			unit := sensor.Unit
			deviceClass := ""
			stateClass := ""

			if isNumeric {
				unit, _ = mqtt.NormalizeUnit(sensor.Unit)
				if dc, ok := mqtt.DeviceClasses[unit]; ok {
					deviceClass = dc
				}
				if sc, ok := mqtt.StateClasses[unit]; ok {
					stateClass = sc
				}
				if unit == "kWh" || unit == "Wh" {
					stateClass = "total_increasing"
				}
			}

			// Power factor has an empty unit from WiNet but needs HA metadata.
			if sensorSlug == "total_power_factor" {
				deviceClass = "power_factor"
				unit = " "
			}

			// Normalize value the same way the publisher would.
			value := sensor.Value
			if isNumeric {
				_, multiplier := mqtt.NormalizeUnit(sensor.Unit)
				if multiplier != 1 {
					if v, err := strconv.ParseFloat(sensor.Value, 64); err == nil {
						value = fmt.Sprintf("%g", v*multiplier)
					}
				}
			}

			snap.Sensors = append(snap.Sensors, snapshot.SensorEntry{
				DeviceSlug:  deviceSlug,
				SensorSlug:  sensorSlug,
				SensorName:  sensor.Name,
				Value:       value,
				Unit:        unit,
				DeviceClass: deviceClass,
				StateClass:  stateClass,
				IsNumeric:   isNumeric,
			})
		}
	}

	data, err := snap.JSON()
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	if output != "" {
		if err := os.WriteFile(output, data, 0600); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Wrote %d devices, %d sensors to %s\n", len(snap.Devices), len(snap.Sensors), output)
	} else {
		fmt.Println(string(data))
	}

	return nil
}

func parseNumeric(value, unit string) (float64, bool) {
	if unit == "" {
		return 0, false
	}
	if _, ok := winet.NumericUnits[unit]; !ok {
		return 0, false
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
