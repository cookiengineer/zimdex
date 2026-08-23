package zim

import "strings"

func split_zim_path(path string) (string, string, bool) {

	trimmed := strings.TrimPrefix(path, "/")

	if trimmed != "" {

		index    := strings.Index(trimmed, "/")
		zim_file := trimmed
		zim_path := ""

		if index != -1 {
			zim_file = trimmed[:index]
			zim_path = trimmed[index+1:]
		}

		if strings.HasSuffix(zim_file, ".zim") == true {
			return zim_file, zim_path, true
		} else {
			return "", "", false
		}

	} else {
		return "", "", false
	}


}

