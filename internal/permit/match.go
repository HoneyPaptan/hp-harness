package permit

func Match(pattern, s string) bool {
	return matchAt(pattern, s, 0, 0)
}

func matchAt(pattern, s string, pi, si int) bool {
	for pi < len(pattern) {
		if pattern[pi] == '*' {
			if pi == len(pattern)-1 {
				return true
			}
			for si <= len(s) {
				if matchAt(pattern, s, pi+1, si) {
					return true
				}
				si++
			}
			return false
		}
		if si >= len(s) || pattern[pi] != s[si] {
			return false
		}
		pi++
		si++
	}
	return si == len(s)
}
