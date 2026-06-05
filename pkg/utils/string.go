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

func IndexValid(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	return page, limit
}