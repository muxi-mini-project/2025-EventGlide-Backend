package tools

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func GenUUID(c *gin.Context) string {
	u, _ := uuid.NewUUID()
	return u.String()
}

func Marshal(v interface{}) string {
	res, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(res)
}

func Unmarshal(data []byte, v interface{}) interface{} {
	err := json.Unmarshal(data, v)
	if err != nil {
		return nil
	}
	return v
}

func ReturnMSG(c *gin.Context, msg string, res ...interface{}) map[string]interface{} {
	return gin.H{
		"code": c.Writer.Status(),
		"msg":  msg,
		"data": res,
	}
}
