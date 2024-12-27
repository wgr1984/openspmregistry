package utils

import (
	"crypto/rand"
	"encoding/base64"
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

func RandomString(i int) (string, error) {
	b := make([]byte, i)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
