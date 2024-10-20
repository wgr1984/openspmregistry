package utils

import (
	"strings"
)

func CopyStruct[T any](src *T) T {
	temp := *src
	temp2 := temp
	return temp2
}

func StripExtension(s string, ext string) string {
	if strings.HasSuffix(s, ext) {
		return strings.TrimSuffix(s, ext)
	} else {
		return s
	}
}
