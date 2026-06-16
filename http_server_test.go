package quickgo

import "testing"

func TestNewHTTPServerAppliesDefaultsWithoutMutatingInput(t *testing.T) {
	config := &HTTPServerConfig{}

	server, err := NewHTTPServer(config)
	if err != nil {
		t.Fatalf("NewHTTPServer should accept minimal config: %v", err)
	}
	if server == nil {
		t.Fatal("NewHTTPServer returned nil server")
	}

	if config.Address != "" {
		t.Fatalf("expected input address to remain empty, got %q", config.Address)
	}
	if config.Port != 0 {
		t.Fatalf("expected input port to remain 0, got %d", config.Port)
	}
	if server.config.Address != "0.0.0.0" {
		t.Fatalf("expected server default address %q, got %q", "0.0.0.0", server.config.Address)
	}
	if server.config.Port != 8080 {
		t.Fatalf("expected server default port %d, got %d", 8080, server.config.Port)
	}
}
