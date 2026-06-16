package gorm

import (
	"net/url"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestBuildMySQLDSNUsesDriverEscaping(t *testing.T) {
	dsn := buildMySQLDSN(MasterConfig{
		Type:     DatabaseTypeMySQL,
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "user",
		Password: "p@ss:word/with/slash",
		Database: "db/name",
		Timezone: "Asia/Shanghai",
		Params: map[string]string{
			"timeout":   "5s",
			"special":   "a&b=c",
			"parseTime": "false",
		},
	})

	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg.User != "user" || cfg.Passwd != "p@ss:word/with/slash" {
		t.Fatalf("unexpected credentials: user=%q password=%q", cfg.User, cfg.Passwd)
	}
	if cfg.DBName != "db/name" {
		t.Fatalf("unexpected database name: %q", cfg.DBName)
	}
	if cfg.Params["special"] != "a&b=c" {
		t.Fatalf("unexpected special param: %q", cfg.Params["special"])
	}
	if cfg.ParseTime {
		t.Fatal("expected parseTime override to be false")
	}
	if got := cfg.Loc.String(); got != "Asia/Shanghai" {
		t.Fatalf("unexpected loc: %q", got)
	}
}

func TestBuildMySQLSlaveDSNUsesDriverEscaping(t *testing.T) {
	dsn := buildMySQLSlaveDSN(SlaveConfig{
		Host:     "127.0.0.1",
		Port:     3307,
		User:     "replica",
		Password: "p@ss:word/with/slash",
		Database: "replica/db",
		Params: map[string]string{
			"special": "a&b=c",
		},
	})

	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}
	if cfg.User != "replica" || cfg.Passwd != "p@ss:word/with/slash" {
		t.Fatalf("unexpected slave credentials: user=%q password=%q", cfg.User, cfg.Passwd)
	}
	if cfg.DBName != "replica/db" {
		t.Fatalf("unexpected slave database name: %q", cfg.DBName)
	}
	if cfg.Params["special"] != "a&b=c" {
		t.Fatalf("unexpected slave special param: %q", cfg.Params["special"])
	}
}

func TestBuildPostgreSQLDSNEncodesCredentialsAndParams(t *testing.T) {
	dsn := buildPostgreSQLDSN(MasterConfig{
		Host:     "localhost",
		Port:     5432,
		User:     "user:name",
		Password: "p@ss/word",
		Database: "app db",
		SSLMode:  "require",
		Params: map[string]string{
			"application_name": "quick go",
		},
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}
	password, _ := u.User.Password()
	if u.User.Username() != "user:name" || password != "p@ss/word" {
		t.Fatalf("unexpected credentials in %q", dsn)
	}
	if u.Path != "/app db" {
		t.Fatalf("unexpected path: %q", u.Path)
	}
	if got := u.Query().Get("application_name"); got != "quick go" {
		t.Fatalf("unexpected application_name: %q", got)
	}
	if got := u.Query().Get("sslmode"); got != "require" {
		t.Fatalf("unexpected sslmode: %q", got)
	}
}

func TestBuildSQLServerDSNEncodesCredentialsAndParams(t *testing.T) {
	dsn := buildSQLServerDSN(MasterConfig{
		Host:     "localhost",
		Port:     1433,
		User:     "user:name",
		Password: "p@ss/word",
		Database: "app db",
		Params: map[string]string{
			"app name": "quick go",
		},
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}
	password, _ := u.User.Password()
	if u.User.Username() != "user:name" || password != "p@ss/word" {
		t.Fatalf("unexpected credentials in %q", dsn)
	}
	if got := u.Query().Get("database"); got != "app db" {
		t.Fatalf("unexpected database: %q", got)
	}
	if got := u.Query().Get("app name"); got != "quick go" {
		t.Fatalf("unexpected app name: %q", got)
	}
}
