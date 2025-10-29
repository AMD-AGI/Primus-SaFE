package stringUtil

import "strconv"

func IsNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}
