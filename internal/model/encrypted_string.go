package model

import (
	"database/sql/driver"
	"log"
	"strings"

	"github.com/raiki02/EG/pkg/encrypt"
)

// decryptErrorLog 加密字段解密失败时的兜底日志，默认走标准库 log，
// 可在启动时通过 SetDecryptErrorLogf 注入统一 logger。
var decryptErrorLog = func(format string, args ...interface{}) {
	log.Printf(format, args...)
}

// SetDecryptErrorLogf 注入解密失败日志函数（如 zap.Warn）。
func SetDecryptErrorLogf(f func(string, ...interface{})) {
	if f != nil {
		decryptErrorLog = f
	}
}

// EncryptedString 字段级加密类型：写库自动加密，读库自动解密。
// 读兼容双读：无 v1: 前缀视为存量明文原样返回；带前缀解密失败保留原值并记录日志。
type EncryptedString string

func (e EncryptedString) Value() (driver.Value, error) {
	if e == "" {
		return "", nil
	}
	enc, err := encrypt.Encrypt(string(e))
	if err != nil {
		return nil, err
	}
	return enc, nil
}

func (e *EncryptedString) Scan(value interface{}) error {
	if value == nil {
		*e = ""
		return nil
	}
	var s string
	switch v := value.(type) {
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		*e = ""
		return nil
	}
	if !strings.HasPrefix(s, encrypt.Prefix) {
		*e = EncryptedString(s)
		return nil
	}
	dec, err := encrypt.Decrypt(s)
	if err != nil {
		decryptErrorLog("encrypted_string: decrypt failed (len=%d): %v", len(s), err)
		// 保留原值：可能是损坏的密文（保留排查线索），也可能是恰好以 v1: 开头的明文
		*e = EncryptedString(s)
		return nil
	}
	*e = EncryptedString(dec)
	return nil
}

func (EncryptedString) GormDataType() string {
	return "varchar(255)"
}
