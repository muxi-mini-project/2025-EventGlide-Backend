package tools

import "github.com/sony/sonyflake"

var sf *sonyflake.Sonyflake

func InitSonyflake() {
	sf = sonyflake.NewSonyflake(sonyflake.Settings{})
}

// GenerateID 生成一个新的雪花ID
func GenerateID() (int64, error) {
	if sf == nil {
		InitSonyflake()
	}
	id, err := sf.NextID()
	return int64(id), err
}

// MustGenerateID 生成ID，错误时panic
func MustGenerateID() int64 {
	id, err := GenerateID()
	if err != nil {
		panic(err)
	}
	return id
}