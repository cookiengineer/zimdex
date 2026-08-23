package zim

import "strings"

func IsPath(path string) (bool) {

	trimmed := strings.TrimPrefix(path, "/")

	if trimmed != "" {

		index    := strings.Index(trimmed, "/")
		zim_file := trimmed

		if index != -1 {
			zim_file = trimmed[:index]
		}

		return strings.HasSuffix(zim_file, ".zim")

	} else {
		return false
	}

}


