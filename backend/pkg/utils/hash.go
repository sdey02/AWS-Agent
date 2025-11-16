package utils

import (
	"crypto/md5"
	"fmt"
)

func HashString(input string) string {
	hash := md5.Sum([]byte(input))
	return fmt.Sprintf("%x", hash)
}
