package plugin

import (
	"strings"
	"testing"
)

func TestAPIKeySecretConfigSemantics(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    string
		wantErr bool
	}{
		{name: "missing uses public default", yaml: "", want: defaultAPIKeySecret},
		{name: "explicit empty disables", yaml: "api_key_secret: \"\"\n", want: ""},
		{name: "explicit default", yaml: "api_key_secret: \"123456\"\n", want: defaultAPIKeySecret},
		{name: "short custom rejected", yaml: "api_key_secret: short\n", wantErr: true},
		{name: "32 bytes accepted", yaml: "api_key_secret: \"" + strings.Repeat("a", 32) + "\"\n", want: strings.Repeat("a", 32)},
		{name: "utf8 counts bytes", yaml: "api_key_secret: \"密密密密密密密密密密密\"\n", want: "密密密密密密密密密密密"},
		{name: "utf8 below 32 bytes rejected", yaml: "api_key_secret: \"密密密密密密密密密密\"\n", wantErr: true},
		{name: "whitespace is not trimmed", yaml: "api_key_secret: \" " + strings.Repeat("b", 30) + " \"\n", want: " " + strings.Repeat("b", 30) + " "},
		{name: "padded default is custom and too short", yaml: "api_key_secret: \" 123456 \"\n", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := parseConfig([]byte(test.yaml))
			if test.wantErr {
				if err == nil {
					t.Fatalf("accepted config with secret %q", config.APIKeySecret)
				}
				return
			}
			if err != nil || config.APIKeySecret != test.want {
				t.Fatalf("secret = %q, err = %v", config.APIKeySecret, err)
			}
		})
	}
}
