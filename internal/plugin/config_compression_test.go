package plugin

import "testing"

func TestDefaultRetentionDays(t *testing.T) {
	config, err := parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.RetentionDays != 365 {
		t.Fatalf("default retention days = %d, want 365", config.RetentionDays)
	}
	if config.FullModeSessionTTLMinutes != defaultFullModeSessionTTLMinutes {
		t.Fatalf("default full-mode session TTL = %d minutes, want %d", config.FullModeSessionTTLMinutes, defaultFullModeSessionTTLMinutes)
	}
}

func TestFullModeSessionTTLConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    int
		wantErr bool
	}{
		{name: "default", want: defaultFullModeSessionTTLMinutes},
		{name: "custom", yaml: "full_mode_session_ttl_minutes: 60\n", want: 60},
		{name: "minimum", yaml: "full_mode_session_ttl_minutes: 1\n", want: 1},
		{name: "maximum", yaml: "full_mode_session_ttl_minutes: 1440\n", want: 1440},
		{name: "zero", yaml: "full_mode_session_ttl_minutes: 0\n", wantErr: true},
		{name: "negative", yaml: "full_mode_session_ttl_minutes: -1\n", wantErr: true},
		{name: "too large", yaml: "full_mode_session_ttl_minutes: 1441\n", wantErr: true},
		{name: "not an integer", yaml: "full_mode_session_ttl_minutes: 15m\n", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := parseConfig([]byte(test.yaml))
			if test.wantErr {
				if err == nil {
					t.Fatalf("accepted invalid full-mode session TTL: %d", config.FullModeSessionTTLMinutes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.FullModeSessionTTLMinutes != test.want {
				t.Fatalf("full-mode session TTL = %d, want %d", config.FullModeSessionTTLMinutes, test.want)
			}
		})
	}
}

func TestResponseCompressionConfig(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		wantEnabled   bool
		wantThreshold int
		wantErr       bool
	}{
		{name: "defaults", wantEnabled: true, wantThreshold: defaultResponseCompressionMinBytes},
		{name: "explicitly disabled", yaml: "response_compression: false\n", wantThreshold: defaultResponseCompressionMinBytes},
		{name: "zero threshold", yaml: "response_compression_min_bytes: 0\n", wantEnabled: true},
		{name: "custom threshold", yaml: "response_compression_min_bytes: 4096\n", wantEnabled: true, wantThreshold: 4096},
		{name: "negative threshold", yaml: "response_compression_min_bytes: -1\n", wantErr: true},
		{name: "threshold too large", yaml: "response_compression_min_bytes: 16777217\n", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := parseConfig([]byte(test.yaml))
			if test.wantErr {
				if err == nil {
					t.Fatalf("accepted response compression threshold %d", config.ResponseCompressionMinBytes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.ResponseCompression != test.wantEnabled || config.ResponseCompressionMinBytes != test.wantThreshold {
				t.Fatalf("compression config = enabled %t, threshold %d", config.ResponseCompression, config.ResponseCompressionMinBytes)
			}
		})
	}
}
