package util

func Intersect(a, b []string) []string {
	if a == nil {
		return b
	} else if b == nil {
		return a
	}

	seen := make(map[string]bool)
	for _, v := range a {
		seen[v] = true
	}
	r := make([]string, 0)
	for _, v := range b {
		if seen[v] {
			r = append(r, v)
		}
	}
	return r
}
