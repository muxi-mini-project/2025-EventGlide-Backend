package tools

import (
	"strings"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

const set = "qwertyuioplkjhgfdsazxcvbnmQWERTYUIOPLKJHGFDSAZXCVBNM"

func GenRandomUsername(sid string) string {
	var s strings.Builder
	if id, err := gonanoid.Generate(set, 6); err == nil {
		s.Write([]byte("用户"))
		s.Write([]byte(id))
		return s.String()
	}
	return sid
}
