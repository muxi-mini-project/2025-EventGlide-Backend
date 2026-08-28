package model

import (
	"strings"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef"

func TestEncryptedStringValue(t *testing.T) {
	t.Setenv("EG_PII_KEY", testKey)
	e := EncryptedString("张三")
	v, err := e.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("Value() type = %T, want string", v)
	}
	if !strings.HasPrefix(s, "v1:") {
		t.Fatalf("Value() = %q, want v1: prefix", s)
	}
	if strings.Contains(s, "张三") {
		t.Fatalf("Value() leaked plaintext: %q", s)
	}
}

func TestEncryptedStringValueEmpty(t *testing.T) {
	t.Setenv("EG_PII_KEY", testKey)
	v, err := EncryptedString("").Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if v != "" {
		t.Fatalf("Value() empty = %q, want empty string", v)
	}
}

func TestEncryptedStringScan(t *testing.T) {
	t.Setenv("EG_PII_KEY", testKey)

	e := EncryptedString("张三")
	v, _ := e.Value()

	var out EncryptedString
	if err := out.Scan(v); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}
	if string(out) != "张三" {
		t.Fatalf("Scan() = %q, want 张三", string(out))
	}
}

func TestEncryptedStringScanPlaintextCompatible(t *testing.T) {
	t.Setenv("EG_PII_KEY", testKey)

	var out EncryptedString
	if err := out.Scan("存量明文数据"); err != nil {
		t.Fatalf("Scan(plaintext) error: %v", err)
	}
	if string(out) != "存量明文数据" {
		t.Fatalf("Scan(plaintext) = %q, want pass through", string(out))
	}
}

func TestEncryptedStringScanNil(t *testing.T) {
	var out EncryptedString
	if err := out.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if string(out) != "" {
		t.Fatalf("Scan(nil) = %q, want empty", string(out))
	}
}

func TestEncryptedStringScanBytes(t *testing.T) {
	t.Setenv("EG_PII_KEY", testKey)
	var out EncryptedString
	if err := out.Scan([]byte("存量明文字节")); err != nil {
		t.Fatalf("Scan([]byte) error: %v", err)
	}
	if string(out) != "存量明文字节" {
		t.Fatalf("Scan([]byte) = %q, want pass through", string(out))
	}
}

func TestEncryptedStringScanCorrupted(t *testing.T) {
	t.Setenv("EG_PII_KEY", testKey)
	var out EncryptedString
	if err := out.Scan("v1:AAA"); err != nil {
		t.Fatalf("Scan(corrupted) should not error, got %v", err)
	}
	if string(out) != "v1:AAA" {
		t.Fatalf("Scan(corrupted) = %q, want original value preserved", string(out))
	}
}

func TestEncryptedStringScanPlaintextWithPrefix(t *testing.T) {
	t.Setenv("EG_PII_KEY", testKey)
	var out EncryptedString
	plain := "v1:恰好以前缀开头的明文"
	if err := out.Scan(plain); err != nil {
		t.Fatalf("Scan(plaintext with prefix) error: %v", err)
	}
	if string(out) != plain {
		t.Fatalf("Scan(plaintext with prefix) = %q, want pass through", string(out))
	}
}
