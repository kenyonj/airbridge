package cast

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	srv := NewServer(8009, "Test Device", "test-uuid-123")
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.port != 8009 {
		t.Errorf("port = %d, want 8009", srv.port)
	}
	if srv.friendlyName != "Test Device" {
		t.Errorf("friendlyName = %s, want Test Device", srv.friendlyName)
	}
	if srv.deviceID != "test-uuid-123" {
		t.Errorf("deviceID = %s, want test-uuid-123", srv.deviceID)
	}
}

func TestServer_OnMediaLoad(t *testing.T) {
	srv := NewServer(8009, "Test", "uuid")
	called := false
	srv.OnMediaLoad = func(url, contentType string) {
		called = true
		if url != "http://test.com/audio.mp3" {
			t.Errorf("url = %s, want http://test.com/audio.mp3", url)
		}
		if contentType != "audio/mpeg" {
			t.Errorf("contentType = %s, want audio/mpeg", contentType)
		}
	}

	// Simulate calling the callback
	if srv.OnMediaLoad != nil {
		srv.OnMediaLoad("http://test.com/audio.mp3", "audio/mpeg")
	}

	if !called {
		t.Error("OnMediaLoad was not called")
	}
}

func TestGenerateSelfSignedCert(t *testing.T) {
	cert, err := generateSelfSignedCert("Test Device")
	if err != nil {
		t.Fatalf("generateSelfSignedCert error: %v", err)
	}
	if len(cert.Certificate) == 0 {
		t.Error("no certificates generated")
	}
	if cert.PrivateKey == nil {
		t.Error("no private key generated")
	}
}
