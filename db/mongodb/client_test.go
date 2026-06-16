package mongodb

import (
	"net/url"
	"testing"
)

func TestBuildURIEncodesCredentialsDatabaseAndOptions(t *testing.T) {
	uri, err := buildURI(&MongoConfig{
		Host:       "localhost",
		Port:       27018,
		Username:   "user:name",
		Password:   "p@ss/word",
		Database:   "app db",
		AuthSource: "admin/db",
		Options: map[string]string{
			"replicaSet":  "rs/0",
			"appName":     "quick go",
			"readConcern": "majority",
		},
	})
	if err != nil {
		t.Fatalf("buildURI failed: %v", err)
	}

	u, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}
	password, _ := u.User.Password()
	if u.User.Username() != "user:name" || password != "p@ss/word" {
		t.Fatalf("unexpected credentials in %q", uri)
	}
	if u.Path != "/app db" {
		t.Fatalf("unexpected database path: %q", u.Path)
	}
	query := u.Query()
	if got := query.Get("authSource"); got != "admin/db" {
		t.Fatalf("unexpected authSource: %q", got)
	}
	if got := query.Get("replicaSet"); got != "rs/0" {
		t.Fatalf("unexpected replicaSet: %q", got)
	}
	if got := query.Get("appName"); got != "quick go" {
		t.Fatalf("unexpected appName: %q", got)
	}
}

func TestBuildURIRequiresHost(t *testing.T) {
	if _, err := buildURI(&MongoConfig{}); err == nil {
		t.Fatal("expected missing host to return an error")
	}
}
