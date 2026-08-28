package model

import (
	"database/sql/driver"
	"fmt"
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
		// 返回错误中止读取，避免密文进入业务值后被读后写（Save）双重加密造成数据损坏。
		// wrap ErrDecrypt 供上层 errors.Is 识别"数据损坏"，避免误报为"记录不存在"。
		return fmt.Errorf("%w (len=%d): %v", encrypt.ErrDecrypt, len(s), err)
	}
	*e = EncryptedString(dec)
	return nil
}

func (EncryptedString) GormDataType() string {
	return "varchar(255)"
}
