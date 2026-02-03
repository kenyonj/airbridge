package ssdp

import (
	"testing"
)

func TestConstants(t *testing.T) {
	// Verify SSDP constants are properly defined
	if DeviceType != "urn:schemas-upnp-org:device:MediaRenderer:1" {
		t.Errorf("DeviceType = %q, want MediaRenderer type", DeviceType)
	}
	if AVTransportType != "urn:schemas-upnp-org:service:AVTransport:1" {
		t.Errorf("AVTransportType = %q, want AVTransport type", AVTransportType)
	}
	if RenderingType != "urn:schemas-upnp-org:service:RenderingControl:1" {
		t.Errorf("RenderingType = %q, want RenderingControl type", RenderingType)
	}
}

func TestHeaderValue(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		key  string
		want string
	}{
		{
			name: "simple header",
			raw:  "HOST: 239.255.255.250:1900\r\nST: ssdp:all\r\n",
			key:  "ST",
			want: "ssdp:all",
		},
		{
			name: "case insensitive",
			raw:  "host: 239.255.255.250:1900\r\nst: upnp:rootdevice\r\n",
			key:  "ST",
			want: "upnp:rootdevice",
		},
		{
			name: "with extra whitespace",
			raw:  "ST:   ssdp:all   \r\n",
			key:  "ST",
			want: "ssdp:all",
		},
		{
			name: "MAN header with quotes",
			raw:  "MAN: \"ssdp:discover\"\r\nST: ssdp:all\r\n",
			key:  "MAN",
			want: "\"ssdp:discover\"",
		},
		{
			name: "header not found",
			raw:  "HOST: 239.255.255.250:1900\r\n",
			key:  "ST",
			want: "",
		},
		{
			name: "empty raw",
			raw:  "",
			key:  "ST",
			want: "",
		},
		{
			name: "M-SEARCH request",
			raw: "M-SEARCH * HTTP/1.1\r\n" +
				"HOST: 239.255.255.250:1900\r\n" +
				"MAN: \"ssdp:discover\"\r\n" +
				"MX: 3\r\n" +
				"ST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n",
			key:  "ST",
			want: "urn:schemas-upnp-org:device:MediaRenderer:1",
		},
		{
			name: "get MX value",
			raw: "M-SEARCH * HTTP/1.1\r\n" +
				"HOST: 239.255.255.250:1900\r\n" +
				"MX: 5\r\n" +
				"ST: ssdp:all\r\n",
			key:  "MX",
			want: "5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headerValue(tt.raw, tt.key)
			if got != tt.want {
				t.Errorf("headerValue() = %q, want %q", got, tt.want)
			}
		})
	}
}
