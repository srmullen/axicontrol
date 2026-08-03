// Package config loads axicontrol's runtime configuration from environment
// variables, with defaults suited to running locally outside a container.
package config

import (
	"os"
	"path/filepath"
)

const (
	defaultDataDir = "./data"
	defaultPort    = "8080"
)

// Config holds the environment-derived settings the service needs to start.
type Config struct {
	// DevicePath is the path to the AxiDraw device (e.g. /dev/axidraw). Empty
	// lets axicli auto-detect the device over USB.
	DevicePath string
	// DataDir is the root of the durable data volume.
	DataDir string
	// DBPath is the path to the SQLite database file.
	DBPath string
	// Port is the TCP port the HTTP server listens on.
	Port string
}

// Load reads Config from environment variables, falling back to
// local-development defaults for anything unset.
func Load() Config {
	dataDir := os.Getenv("AXICONTROL_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}

	dbPath := os.Getenv("AXICONTROL_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "db", "axicontrol.sqlite")
	}

	port := os.Getenv("AXICONTROL_PORT")
	if port == "" {
		port = defaultPort
	}

	return Config{
		DevicePath: os.Getenv("AXICONTROL_DEVICE_PATH"),
		DataDir:    dataDir,
		DBPath:     dbPath,
		Port:       port,
	}
}
