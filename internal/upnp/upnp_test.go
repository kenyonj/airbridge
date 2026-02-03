package upnp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseSOAPAction(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"with quotes and hash", `"urn:schemas-upnp-org:service:AVTransport:1#Play"`, "Play"},
		{"no quotes", "urn:schemas-upnp-org:service:AVTransport:1#Stop", "Stop"},
		{"just action", "Play", "Play"},
		{"empty", "", ""},
		{"with namespace prefix", `"urn:schemas-upnp-org:service:RenderingControl:1#SetVolume"`, "SetVolume"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSOAPAction(tt.header)
			if got != tt.want {
				t.Errorf("ParseSOAPAction(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestXMLText(t *testing.T) {
	tests := []struct {
		name string
		xml  string
		tag  string
		want string
	}{
		{
			name: "simple tag",
			xml:  `<CurrentURI>http://example.com/audio.mp3</CurrentURI>`,
			tag:  "CurrentURI",
			want: "http://example.com/audio.mp3",
		},
		{
			name: "with namespace prefix",
			xml:  `<u:DesiredVolume>80</u:DesiredVolume>`,
			tag:  "DesiredVolume",
			want: "80",
		},
		{
			name: "with whitespace",
			xml:  `<CurrentURI>  http://example.com/audio.mp3  </CurrentURI>`,
			tag:  "CurrentURI",
			want: "http://example.com/audio.mp3",
		},
		{
			name: "with HTML entities",
			xml:  `<CurrentURI>http://example.com/audio.mp3?foo=1&amp;bar=2</CurrentURI>`,
			tag:  "CurrentURI",
			want: "http://example.com/audio.mp3?foo=1&bar=2",
		},
		{
			name: "nested in envelope",
			xml: `<?xml version="1.0"?>
				<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
					<s:Body>
						<u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
							<InstanceID>0</InstanceID>
							<CurrentURI>http://example.com/song.flac</CurrentURI>
						</u:SetAVTransportURI>
					</s:Body>
				</s:Envelope>`,
			tag:  "CurrentURI",
			want: "http://example.com/song.flac",
		},
		{
			name: "tag not found",
			xml:  `<OtherTag>value</OtherTag>`,
			tag:  "CurrentURI",
			want: "",
		},
		{
			name: "empty tag",
			xml:  `<CurrentURI></CurrentURI>`,
			tag:  "CurrentURI",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := XMLText([]byte(tt.xml), tt.tag)
			if got != tt.want {
				t.Errorf("XMLText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteSOAPResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteSOAPResponse(rec, AVTransportType, "PlayResponse", "<Result>OK</Result>")

	resp := rec.Result()
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/xml") {
		t.Errorf("expected Content-Type text/xml, got %q", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "PlayResponse") {
		t.Error("response should contain PlayResponse")
	}
	if !strings.Contains(body, AVTransportType) {
		t.Error("response should contain namespace")
	}
	if !strings.Contains(body, "<Result>OK</Result>") {
		t.Error("response should contain inner content")
	}
	if !strings.Contains(body, `<?xml version="1.0"`) {
		t.Error("response should have XML declaration")
	}
}

func TestWriteSOAPError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteSOAPError(rec, 401, "Invalid Action")

	resp := rec.Result()
	if resp.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", resp.StatusCode)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "401") {
		t.Error("response should contain error code 401")
	}
	if !strings.Contains(body, "Invalid Action") {
		t.Error("response should contain error description")
	}
	if !strings.Contains(body, "s:Fault") {
		t.Error("response should contain SOAP Fault element")
	}
}

func TestControllerID(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ip with port", "192.168.1.100:54321", "192.168.1.100"},
		{"ipv6 with port", "[::1]:54321", "::1"},
		{"ip only", "192.168.1.100", "192.168.1.100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{RemoteAddr: tt.remoteAddr}
			got := ControllerID(req)
			if got != tt.want {
				t.Errorf("ControllerID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConnectionManagerHandler(t *testing.T) {
	handler := ConnectionManagerHandler()

	tests := []struct {
		name       string
		action     string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "GetProtocolInfo",
			action:     `"urn:schemas-upnp-org:service:ConnectionManager:1#GetProtocolInfo"`,
			wantStatus: 200,
			wantBody:   "audio/mpeg",
		},
		{
			name:       "GetCurrentConnectionIDs",
			action:     `"urn:schemas-upnp-org:service:ConnectionManager:1#GetCurrentConnectionIDs"`,
			wantStatus: 200,
			wantBody:   "ConnectionIDs",
		},
		{
			name:       "GetCurrentConnectionInfo",
			action:     `"urn:schemas-upnp-org:service:ConnectionManager:1#GetCurrentConnectionInfo"`,
			wantStatus: 200,
			wantBody:   "Direction",
		},
		{
			name:       "Invalid action",
			action:     `"urn:schemas-upnp-org:service:ConnectionManager:1#InvalidAction"`,
			wantStatus: 500,
			wantBody:   "Invalid Action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/control", nil)
			req.Header.Set("SOAPACTION", tt.action)
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body should contain %q, got %s", tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestUPnPTypeConstants(t *testing.T) {
	// Verify UPnP type constants are properly defined
	if DeviceType != "urn:schemas-upnp-org:device:MediaRenderer:1" {
		t.Errorf("DeviceType = %q, want MediaRenderer type", DeviceType)
	}
	if AVTransportType != "urn:schemas-upnp-org:service:AVTransport:1" {
		t.Errorf("AVTransportType = %q, want AVTransport type", AVTransportType)
	}
	if RenderingType != "urn:schemas-upnp-org:service:RenderingControl:1" {
		t.Errorf("RenderingType = %q, want RenderingControl type", RenderingType)
	}
	if ConnectionType != "urn:schemas-upnp-org:service:ConnectionManager:1" {
		t.Errorf("ConnectionType = %q, want ConnectionManager type", ConnectionType)
	}
}
