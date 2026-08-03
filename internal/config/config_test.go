package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		devicePath string
		dataDir    string
		dbPath     string
		port       string
		want       Config
	}{
		{
			name: "defaults",
			want: Config{DevicePath: "", DataDir: "./data", DBPath: "data/db/axicontrol.sqlite", Port: "8080"},
		},
		{
			name:       "values from env",
			devicePath: "/dev/axidraw",
			dataDir:    "/data",
			dbPath:     "/data/db/custom.sqlite",
			port:       "9090",
			want:       Config{DevicePath: "/dev/axidraw", DataDir: "/data", DBPath: "/data/db/custom.sqlite", Port: "9090"},
		},
		{
			name:    "db path defaults under a custom data dir",
			dataDir: "/srv/axicontrol",
			want:    Config{DevicePath: "", DataDir: "/srv/axicontrol", DBPath: "/srv/axicontrol/db/axicontrol.sqlite", Port: "8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AXICONTROL_DEVICE_PATH", tt.devicePath)
			t.Setenv("AXICONTROL_DATA_DIR", tt.dataDir)
			t.Setenv("AXICONTROL_DB_PATH", tt.dbPath)
			t.Setenv("AXICONTROL_PORT", tt.port)

			assert.Equal(t, tt.want, Load())
		})
	}
}
