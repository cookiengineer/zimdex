package filters

func Get(names []string) []Filter {

	result := make([]Filter, 0)
	wanted := make(map[string]bool, len(names))

	for _, name := range names {
		wanted[name] = true
	}

	for _, filter := range Registry {

		if wanted[filter.Name()] == true {
			result = append(result, filter)
		}

	}

	return result

}
