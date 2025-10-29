package stringUtil

func In(s string, strs []string) bool {
	for _, str := range strs {
		if s == str {
			return true
		}
	}
	return false
}

func SliceEqual(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	sliceValues := make(map[string]struct{})
	for _, s := range s1 {
		sliceValues[s] = struct{}{}
	}
	for _, s := range s2 {
		if _, ok := sliceValues[s]; !ok {
			return false
		}
	}
	return true
}

// IsSubset returns true if all items in slice A are present in slice B.
func IsSubset(a, b []string) bool {
	set := make(map[string]bool, len(b))
	for _, v := range b {
		set[v] = true
	}
	for _, v := range a {
		if _, ok := set[v]; !ok {
			return false
		}
	}

	return true
}
