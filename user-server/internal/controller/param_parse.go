package controller

import "strconv"

func parsePositiveInt(s string, def, max int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
