package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	Prefix = "v1:"
	EnvKey = "EG_PII_KEY"
)

var errKeyMissing = fmt.Errorf("环境变量 %s 未设置", EnvKey)

// ErrDecrypt 加密字段解密失败的哨兵错误，供上层 errors.Is 区分"数据损坏"与其他错误。
var ErrDecrypt = errors.New("encrypt: decrypt failed")

func Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	key, err := loadKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return Prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密带 v1: 前缀的密文。非密文（无前缀）返回错误，
// 明文兼容逻辑由调用方（EncryptedString.Scan）判断前缀处理。
func Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, Prefix) {
		return "", errors.New("encrypt: missing v1: prefix")
	}
	key, err := loadKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, Prefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid ciphertext")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func loadKey() ([]byte, error) {
	k := os.Getenv(EnvKey)
	if k == "" {
		return nil, errKeyMissing
	}
	h := sha256.Sum256([]byte(k))
	return h[:], nil
}

// ValidateKey 启动时校验加密密钥已配置，避免运行时写路径静默失败。
func ValidateKey() error {
	_, err := loadKey()
	return err
}
