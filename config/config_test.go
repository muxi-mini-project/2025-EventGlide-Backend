package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestParseNacosDSN(t *testing.T) {
	t.Setenv(EgConf, "127.0.0.1:8848?namespace=ns1&username=user1&password=pass1&group=grp1&dataId=cfg1")

	server, port, ns, user, pass, group, dataId, err := parseNacosDSN(EgConf)
	if err != nil {
		t.Fatalf("parseNacosDSN error: %v", err)
	}
	if server != "127.0.0.1" {
		t.Fatalf("server = %q", server)
	}
	if port != 8848 {
		t.Fatalf("port = %d", port)
	}
	if ns != "ns1" {
		t.Fatalf("ns = %q", ns)
	}
	if user != "user1" || pass != "pass1" {
		t.Fatalf("user/pass = %q/%q", user, pass)
	}
	if group != "grp1" || dataId != "cfg1" {
		t.Fatalf("group/dataId = %q/%q", group, dataId)
	}
}

func TestParseNacosDSNDefaults(t *testing.T) {
	t.Setenv(EgConf, "nacos.example.com")

	server, port, ns, _, _, group, dataId, err := parseNacosDSN(EgConf)
	if err != nil {
		t.Fatalf("parseNacosDSN error: %v", err)
	}
	if server != "nacos.example.com" {
		t.Fatalf("server = %q", server)
	}
	if port != 8848 {
		t.Fatalf("default port = %d", port)
	}
	if ns != "public" {
		t.Fatalf("default ns = %q", ns)
	}
	if group != "" || dataId != "" {
		t.Fatalf("empty group/dataId = %q/%q", group, dataId)
	}
}

func TestParseNacosDSNMissingEnv(t *testing.T) {
	t.Setenv(EgConf, "")
	if _, _, _, _, _, _, _, err := parseNacosDSN(EgConf); err == nil {
		t.Fatal("expected error when env missing")
	}
}

func TestUnmarshalConf(t *testing.T) {
	content := `
mysql:
  dsn: "user:pass@tcp(127.0.0.1:3306)/eg?charset=utf8mb4"
  maxIdleConns: 20
  maxOpenConns: 10
redis:
  addr: "127.0.0.1:6379"
  password: "rpass"
jwt:
  key: "jwt-secret"
  ttl: 259200
imgbed:
  accessKey: "ak"
  secretKey: "sk"
  bucket: "bucket"
auditor:
  hookUrl: "http://localhost:8081"
  apiKey: "apikey"
  effect: "slow"
kafka:
  addr: "127.0.0.1:9092"
log:
  path: "./log"
  maxSize: 100
shenlongConf:
  api: "http://shenlong"
  interval: 67
  retry: 3
piiKey: "test-pii-key"
`
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewBufferString(content)); err != nil {
		t.Fatalf("ReadConfig error: %v", err)
	}

	var c Conf
	if err := v.Unmarshal(&c); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if c.Mysql.DSN != "user:pass@tcp(127.0.0.1:3306)/eg?charset=utf8mb4" {
		t.Fatalf("mysql.dsn = %q", c.Mysql.DSN)
	}
	if c.Mysql.MaxIdleConns != 20 || c.Mysql.MaxOpenConns != 10 {
		t.Fatalf("mysql conns = %d/%d", c.Mysql.MaxIdleConns, c.Mysql.MaxOpenConns)
	}
	if c.Redis.Addr != "127.0.0.1:6379" {
		t.Fatalf("redis.addr = %q", c.Redis.Addr)
	}
	if c.JWT.Key != "jwt-secret" || c.JWT.Ttl != 259200 {
		t.Fatalf("jwt = %q/%d", c.JWT.Key, c.JWT.Ttl)
	}
	if c.Auditor.Effect != "slow" {
		t.Fatalf("auditor.effect = %q", c.Auditor.Effect)
	}
	if c.ShenlongConf.Interval != 67 || c.ShenlongConf.Retry != 3 {
		t.Fatalf("shenlong = %d/%d", c.ShenlongConf.Interval, c.ShenlongConf.Retry)
	}
	if c.PIIKey != "test-pii-key" {
		t.Fatalf("piiKey = %q", c.PIIKey)
	}
}

func TestUnmarshalConfRejectsNonYAML(t *testing.T) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewBufferString("mysql: [not-a-map")); err == nil {
		t.Fatal("expected error for invalid yaml")
	} else if !strings.Contains(err.Error(), "yaml") {
		t.Fatalf("unexpected error: %v", err)
	}
}
