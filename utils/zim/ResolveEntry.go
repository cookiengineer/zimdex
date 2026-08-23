package zim

import "github.com/cookiengineer/gozim/archive/zim"
import "fmt"
import "strings"

func ResolveEntry(archive *zim.Archive, path string) (*zim.Entry, error) {

	variants := []string{path}

	if strings.HasPrefix(path, "C/") == false {
		variants = append(variants, "C/"+path)
	}

	index := strings.Index(path, "/")

	if index > 0 {

		host   := path[:index]
		suffix := path[index:]

		if strings.HasPrefix(host, "www.") == false {
			variants = append(variants, fmt.Sprintf("www.%s%s", host, suffix))
			variants = append(variants, fmt.Sprintf("C/www.%s%s", host, suffix))
		} else {
			no_www := strings.TrimPrefix(host, "www.")
			variants = append(variants, fmt.Sprintf("%s%s", no_www, suffix))
			variants = append(variants, fmt.Sprintf("C/%s%s", no_www, suffix))
		}

		no_root := suffix[1:]
		variants = append(variants, no_root)
		variants = append(variants, fmt.Sprintf("C/%s", no_root))

	}

	for _, variant := range variants {

		entry, err1 := archive.EntryByPath(variant)

		if err1 == nil {

			if entry.IsRedirect() == true {

				// max 10 redirects
				for r := 0; r < 10; r++ {

					redirect_entry, err2 := entry.RedirectEntry()

					if err2 == nil {
						entry = redirect_entry
					} else {
						return nil, fmt.Errorf("invalid redirect at %s: %s", variant, err2.Error())
					}

				}

			}

			if entry.IsRedirect() {
				return nil, fmt.Errorf("too many redirects at %s", variant)
			} else {
				return entry, nil
			}

		} else {
			continue
		}

	}

	return nil, fmt.Errorf("entry not found, tried %v", variants)

}
