package redis

import "testing"

func TestBuildTLSConfig(t *testing.T) {
	tlsConfig, err := buildTLSConfig(&RedisConfig{TLS: true}, "redis.internal:6380")
	if err != nil {
		t.Fatalf("buildTLSConfig failed: %v", err)
	}
	if tlsConfig == nil || tlsConfig.ServerName != "redis.internal" {
		t.Fatalf("unexpected TLS config: %#v", tlsConfig)
	}
}

func TestBuildTLSConfigRejectsPartialClientCertificate(t *testing.T) {
	_, err := buildTLSConfig(&RedisConfig{TLS: true, TLSCertFile: "client.pem"}, "redis.internal:6380")
	if err == nil {
		t.Fatal("expected partial client certificate configuration to fail")
	}
}

func TestBuildTLSConfigRequiresTLSFlag(t *testing.T) {
	_, err := buildTLSConfig(&RedisConfig{TLSCAFile: "ca.pem"}, "redis.internal:6380")
	if err == nil {
		t.Fatal("expected TLS options without tls=true to fail")
	}
}
