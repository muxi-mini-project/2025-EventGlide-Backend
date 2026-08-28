package encrypt

import (
	"strings"
	"testing"
)

const testKey = "0123456789abcdef0123456789abcdef"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv(EnvKey, testKey)
	for _, plain := range []string{"张三", "李四", "very long name with spaces and symbols 中文混合 abc123", "单字"} {
		enc, err := Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q) error: %v", plain, err)
		}
		if !strings.HasPrefix(enc, Prefix) {
			t.Fatalf("Encrypt(%q) = %q, want prefix %q", plain, enc, Prefix)
		}
		dec, err := Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt(%q) error: %v", enc, err)
		}
		if dec != plain {
			t.Fatalf("round trip mismatch: got %q want %q", dec, plain)
		}
	}
}

func TestEncryptProducesRandomCiphertext(t *testing.T) {
	t.Setenv(EnvKey, testKey)
	a, _ := Encrypt("张三")
	b, _ := Encrypt("张三")
	if a == b {
		t.Fatalf("same plaintext should produce different ciphertext, got %q == %q", a, b)
	}
}

func TestDecryptMissingPrefix(t *testing.T) {
	t.Setenv(EnvKey, testKey)
	if _, err := Decrypt("存量明文数据"); err == nil {
		t.Fatal("Decrypt without v1: prefix should error")
	}
}

func TestDecryptWrongKey(t *testing.T) {
	t.Setenv(EnvKey, testKey)
	enc, _ := Encrypt("张三")

	t.Setenv(EnvKey, strings.Repeat("f", 32))
	if _, err := Decrypt(enc); err == nil {
		t.Fatal("Decrypt with wrong key should error")
	}
}

func TestAnyLengthKey(t *testing.T) {
	t.Setenv(EnvKey, "short-key")
	enc, err := Encrypt("张三")
	if err != nil {
		t.Fatalf("Encrypt with short key error: %v", err)
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt with short key error: %v", err)
	}
	if dec != "张三" {
		t.Fatalf("round trip = %q", dec)
	}
}

func TestSetKeyTakesPrecedenceOverEnv(t *testing.T) {
	t.Setenv(EnvKey, "env-key")
	SetKey("nacos-key")
	defer SetKey("")

	enc, err := Encrypt("张三")
	if err != nil {
		t.Fatal(err)
	}

	// 清空注入后走 env-key，必须解不开 nacos-key 的密文，证明加密用的是注入的 key
	SetKey("")
	if _, err := Decrypt(enc); err == nil {
		t.Fatal("ciphertext from injected key should not decrypt with env key")
	}

	SetKey("nacos-key")
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != "张三" {
		t.Fatalf("round trip = %q", dec)
	}
}

func TestSetKeyEmptyFallsBackToEnv(t *testing.T) {
	t.Setenv(EnvKey, "env-key")
	SetKey("nacos-key")
	SetKey("")

	enc, err := Encrypt("张三")
	if err != nil {
		t.Fatalf("Encrypt after clearing SetKey error: %v", err)
	}
	dec, err := Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != "张三" {
		t.Fatalf("round trip = %q", dec)
	}

	// 换成其他注入 key 后必须解不开 env-key 的密文，证明清空后走的是 env-key
	SetKey("other-key")
	if _, err := Decrypt(enc); err == nil {
		t.Fatal("ciphertext from env key should not decrypt with different injected key")
	}
}

func TestSetKeyThenEnvKeyChangeBreaksDecrypt(t *testing.T) {
	SetKey("key-a")
	defer SetKey("")
	enc, err := Encrypt("张三")
	if err != nil {
		t.Fatal(err)
	}

	SetKey("key-b")
	if _, err := Decrypt(enc); err == nil {
		t.Fatal("Decrypt with different injected key should error")
	}
}

func TestEncryptEmpty(t *testing.T) {
	t.Setenv(EnvKey, testKey)
	enc, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty error: %v", err)
	}
	if enc != "" {
		t.Fatalf("Encrypt empty = %q, want empty", enc)
	}
}

func TestDecryptInvalidCiphertext(t *testing.T) {
	t.Setenv(EnvKey, testKey)
	cases := []string{
		"v1:!!!not-base64!!!",
		"v1:c2hvcnQ=",
		"v1:" + strings.Repeat("AA", 64),
	}
	for _, c := range cases {
		if _, err := Decrypt(c); err == nil {
			t.Fatalf("Decrypt(%q) should error", c)
		}
	}
}

func TestKeyMissing(t *testing.T) {
	t.Setenv(EnvKey, "")
	if _, err := Encrypt("张三"); err == nil {
		t.Fatal("Encrypt should error when key missing")
	}
	if _, err := Decrypt("v1:AAA"); err == nil {
		t.Fatal("Decrypt should error when key missing")
	}
}
