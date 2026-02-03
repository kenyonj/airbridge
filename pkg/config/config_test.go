package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HTTPPort != 8200 {
		t.Errorf("expected HTTPPort 8200, got %d", cfg.HTTPPort)
	}
	if !cfg.AutoDiscover {
		t.Error("expected AutoDiscover to be true")
	}
	if cfg.NamePrefix != "Airbridge" {
		t.Errorf("expected NamePrefix 'Airbridge', got %q", cfg.NamePrefix)
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantPort int
		wantAuto bool
		wantErr  bool
	}{
		{
			name:     "empty file uses defaults",
			yaml:     "",
			wantPort: 8200,
			wantAuto: true,
		},
		{
			name:     "custom port",
			yaml:     "http_port: 9000",
			wantPort: 9000,
			wantAuto: true,
		},
		{
			name:     "disable auto discover",
			yaml:     "auto_discover: false",
			wantPort: 8200,
			wantAuto: false,
		},
		{
			name: "full config",
			yaml: `
http_port: 8500
auto_discover: false
name_prefix: "Test"
devices:
  - name: "Living Room"
    alias: "LR Speaker"
    port: 8501
    volume: 80
`,
			wantPort: 8500,
			wantAuto: false,
		},
		{
			name:    "invalid yaml",
			yaml:    "http_port: [invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatalf("failed to write temp config: %v", err)
			}

			cfg, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if cfg.HTTPPort != tt.wantPort {
				t.Errorf("HTTPPort = %d, want %d", cfg.HTTPPort, tt.wantPort)
			}
			if cfg.AutoDiscover != tt.wantAuto {
				t.Errorf("AutoDiscover = %v, want %v", cfg.AutoDiscover, tt.wantAuto)
			}
		})
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	// Should return defaults
	if cfg.HTTPPort != 8200 {
		t.Errorf("expected default HTTPPort 8200, got %d", cfg.HTTPPort)
	}
}

func TestGetDeviceConfig(t *testing.T) {
	enabled := true
	cfg := Config{
		Devices: []DeviceConfig{
			{Name: "Living Room", Alias: "LR", Port: 8501, Volume: 80, Enabled: &enabled},
			{Name: "Kitchen", Alias: "Kit"},
		},
	}

	tests := []struct {
		name      string
		device    string
		wantAlias string
		wantNil   bool
	}{
		{"existing device", "Living Room", "LR", false},
		{"another device", "Kitchen", "Kit", false},
		{"non-existent device", "Bedroom", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := cfg.GetDeviceConfig(tt.device)
			if tt.wantNil {
				if dc != nil {
					t.Errorf("expected nil, got %+v", dc)
				}
				return
			}
			if dc == nil {
				t.Fatal("expected device config, got nil")
			}
			if dc.Alias != tt.wantAlias {
				t.Errorf("Alias = %q, want %q", dc.Alias, tt.wantAlias)
			}
		})
	}
}

func TestShouldIncludeDevice(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name   string
		cfg    Config
		device string
		want   bool
	}{
		{
			name:   "auto discover includes all",
			cfg:    Config{AutoDiscover: true},
			device: "Any Device",
			want:   true,
		},
		{
			name:   "auto discover off excludes all",
			cfg:    Config{AutoDiscover: false},
			device: "Any Device",
			want:   false,
		},
		{
			name: "explicit enabled device",
			cfg: Config{
				AutoDiscover: false,
				Devices:      []DeviceConfig{{Name: "Living Room", Enabled: &enabled}},
			},
			device: "Living Room",
			want:   true,
		},
		{
			name: "explicit disabled device",
			cfg: Config{
				AutoDiscover: true,
				Devices:      []DeviceConfig{{Name: "Living Room", Enabled: &disabled}},
			},
			device: "Living Room",
			want:   false,
		},
		{
			name: "device in filter",
			cfg: Config{
				AutoDiscover: true,
				DeviceFilter: []string{"Living Room", "Kitchen"},
			},
			device: "Kitchen",
			want:   true,
		},
		{
			name: "device not in filter",
			cfg: Config{
				AutoDiscover: true,
				DeviceFilter: []string{"Living Room"},
			},
			device: "Bedroom",
			want:   false,
		},
		{
			name: "wildcard suffix filter",
			cfg: Config{
				DeviceFilter: []string{"Living*"},
			},
			device: "Living Room",
			want:   true,
		},
		{
			name: "wildcard prefix filter",
			cfg: Config{
				DeviceFilter: []string{"*Speaker"},
			},
			device: "Kitchen Speaker",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.ShouldIncludeDevice(tt.device)
			if got != tt.want {
				t.Errorf("ShouldIncludeDevice(%q) = %v, want %v", tt.device, got, tt.want)
			}
		})
	}
}

func TestMatchFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   bool
	}{
		{"exact match", "Living Room", true},
		{"no match", "Kitchen", false},
		{"prefix wildcard", "Living*", true},
		{"suffix wildcard", "*Room", true},
		{"suffix no match", "*Kitchen", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchFilter("Living Room", tt.filter)
			if got != tt.want {
				t.Errorf("matchFilter(%q, %q) = %v, want %v", "Living Room", tt.filter, got, tt.want)
			}
		})
	}
}
