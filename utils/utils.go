package utils

import (
	"github.com/hashicorp/go-version"
	"slices"
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

func SortVersions(versions []string) []string {
	slices.SortFunc(versions, func(a string, b string) int {
		v1, err := version.NewVersion(a)
		if err != nil {
			return 0
		}
		v2, err := version.NewVersion(b)
		if err != nil {
			return 0
		}
		return v2.Compare(v1)
	})
	return versions
}
