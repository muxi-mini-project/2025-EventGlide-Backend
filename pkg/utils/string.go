package utils

import "strings"

func HasLike(likes string, bid string) bool {
	if likes == "" || bid == "" {
		return false
	}
	parts := strings.Split(likes, ",")
	for _, p := range parts {
		if p == bid {
			return true
		}
	}
	return false
}