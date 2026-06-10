package utils

import (
	"strconv"

	"github.com/pkg/errors"
)

// SnowflakeID 封装 int64，用字符串传输避免 JS 精度丢失
type SnowflakeID int64

func (s SnowflakeID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + strconv.FormatInt(int64(s), 10) + `"`), nil
}

func (s *SnowflakeID) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return errors.New("invalid snowflake id: must be quoted string")
	}
	id, err := strconv.ParseInt(string(data[1:len(data)-1]), 10, 64)
	if err != nil {
		return errors.Wrap(err, "parse snowflake id")
	}
	*s = SnowflakeID(id)
	return nil
}