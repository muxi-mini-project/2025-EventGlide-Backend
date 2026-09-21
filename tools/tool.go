package tools

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

const ab = "qwertyuiopasdfghjklzxcvbnmQWERTYUIOPASDFGHJKLZXCVBNM"

func GenUUID() string {
	return uuid.New().String()
}

func SliceToString(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return strings.Join(s, ",")
}

func StringToSlice(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func ReturnMSG(code int, msg string, res interface{}) map[string]interface{} {
	return gin.H{
		"code": code,
		"msg":  msg,
		"data": res,
	}
}

func ParseTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// StatusMapper 把审核平台的回调状态映射为 auditor_form.status 的枚举值。
// 平台人工审核回调发送中文（未审核/通过/不通过），AI 审核回调发送英文
// （Pending/Pass/Reject），两者都要识别且不区分大小写。无法识别时返回空串，
// 交由调用方拒绝，避免将非法值写入 enum 列。
func StatusMapper(auditStatus string) string {
	switch {
	case auditStatus == "未审核" || strings.EqualFold(auditStatus, "Pending"):
		return "pending"
	case auditStatus == "通过" || strings.EqualFold(auditStatus, "Pass"):
		return "pass"
	case auditStatus == "不通过" || strings.EqualFold(auditStatus, "Reject"):
		return "reject"
	default:
		return ""
	}
}

func RandomMD5() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type Claims struct {
	*jwt.RegisteredClaims
	Random string `json:"random,omitempty"`
}

func GenerateRand4() string {
	return gonanoid.MustGenerate(ab, 4)
}

func SignRandJwt(studentId string) (string, error) {
	now := time.Now()
	c := &Claims{
		&jwt.RegisteredClaims{
			Subject:   studentId,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
		GenerateRand4(),
	}

	secret := GenerateRand4()

	t := jwt.NewWithClaims(jwt.SigningMethodHS512, c)

	j, err := t.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return j, nil
}
