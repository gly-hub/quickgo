package gorm

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
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

func TestNewClientWithReadReplicaKeepsSourceOpen(t *testing.T) {
	dir := t.TempDir()
	client, err := NewClient(&GormConfig{
		Name: "test",
		Master: MasterConfig{
			Type:     DatabaseTypeSQLite,
			Database: filepath.Join(dir, "master.db"),
		},
		Slaves: []SlaveConfig{
			{Database: filepath.Join(dir, "replica.db")},
		},
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	if err := client.GetDB().Exec("CREATE TABLE messages (id integer primary key, body text)").Error; err != nil {
		t.Fatalf("expected source connection to remain usable after replica setup, got %v", err)
	}
}

func TestClientCloseClosesSourceAndReplicaPools(t *testing.T) {
	dir := t.TempDir()
	client, err := NewClient(&GormConfig{
		Name: "test-close",
		Master: MasterConfig{
			Type:     DatabaseTypeSQLite,
			Database: filepath.Join(dir, "master.db"),
		},
		Slaves: []SlaveConfig{{Database: filepath.Join(dir, "replica.db")}},
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	plugin, ok := client.db.Config.Plugins["gorm:db_resolver"].(*dbresolver.DBResolver)
	if !ok {
		t.Fatal("dbresolver plugin was not registered")
	}
	var pools []*sql.DB
	if err := plugin.Call(func(pool gorm.ConnPool) error {
		if sqlDB, ok := pool.(*sql.DB); ok {
			pools = append(pools, sqlDB)
		}
		return nil
	}); err != nil {
		t.Fatalf("collect pools: %v", err)
	}
	if len(pools) < 2 {
		t.Fatalf("expected source and replica pools, got %d", len(pools))
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	for _, pool := range pools {
		if err := pool.Ping(); err == nil {
			t.Fatal("expected closed source/replica pool to reject Ping")
		}
	}
}
