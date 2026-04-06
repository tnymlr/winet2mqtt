package cmd

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:    "health",
	Short:  "Check if the server is healthy (for Docker HEALTHCHECK)",
	Hidden: true,
	RunE:   runHealth,
}

func init() {
	healthCmd.Flags().Int("health-port", 8080, "Health check HTTP port")
	rootCmd.AddCommand(healthCmd)
}

func runHealth(cmd *cobra.Command, _ []string) error {
	bindFlags(cmd)

	port, _ := cmd.Flags().GetInt("health-port")
	url := fmt.Sprintf("http://localhost:%d/healthz", port)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "health check failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "unhealthy: status %d\n", resp.StatusCode)
		os.Exit(1)
	}

	fmt.Println("ok")
	return nil
}
