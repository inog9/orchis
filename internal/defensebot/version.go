package defensebot

import (
	"strconv"
	"strings"
)

// CompareVersions compares two package versions using the rules of the ecosystem. It is used only to pick the applicable
// "fixed" version from an advisory and to compare kernel packages; OSV itself decides whether a version is affected.
// Ecosystems starting with Debian or Ubuntu use the dpkg algorithm; everything else uses a tolerant dotted-number comparison.
func CompareVersions(ecosystem, a, b string) int {
	if strings.HasPrefix(ecosystem, "Debian") || strings.HasPrefix(ecosystem, "Ubuntu") {
		return compareDpkg(a, b)
	}
	return compareLoose(a, b)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func order(c byte) int {
	switch {
	case c == '~':
		return -1
	case isDigit(c):
		return 0
	case isAlpha(c):
		return int(c)
	default:
		return int(c) + 256
	}
}

func verrevcmp(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		first := 0
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			ac, bc := 0, 0
			if i < len(a) {
				ac = order(a[i])
			}
			if j < len(b) {
				bc = order(b[j])
			}
			if ac != bc {
				if ac < bc {
					return -1
				}
				return 1
			}
			if i < len(a) {
				i++
			}
			if j < len(b) {
				j++
			}
		}
		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}
		for i < len(a) && isDigit(a[i]) && j < len(b) && isDigit(b[j]) {
			if first == 0 {
				first = int(a[i]) - int(b[j])
			}
			i++
			j++
		}
		if i < len(a) && isDigit(a[i]) {
			return 1
		}
		if j < len(b) && isDigit(b[j]) {
			return -1
		}
		if first != 0 {
			if first < 0 {
				return -1
			}
			return 1
		}
	}
	return 0
}

func splitDpkg(v string) (epoch int, upstream, revision string) {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, ":"); i >= 0 {
		if n, err := strconv.Atoi(v[:i]); err == nil {
			epoch = n
		}
		v = v[i+1:]
	}
	if i := strings.LastIndex(v, "-"); i >= 0 {
		return epoch, v[:i], v[i+1:]
	}
	return epoch, v, ""
}

func compareDpkg(a, b string) int {
	ea, ua, ra := splitDpkg(a)
	eb, ub, rb := splitDpkg(b)
	if ea != eb {
		if ea < eb {
			return -1
		}
		return 1
	}
	if c := verrevcmp(ua, ub); c != 0 {
		return c
	}
	return verrevcmp(ra, rb)
}

// compareLoose compares dotted versions such as 1.2.10 against 1.2.9, treating a pre-release suffix as lower than the release.
func compareLoose(a, b string) int {
	ca, pa := loosePieces(a)
	cb, pb := loosePieces(b)
	for i := 0; i < len(ca) || i < len(cb); i++ {
		var x, y int
		if i < len(ca) {
			x = ca[i]
		}
		if i < len(cb) {
			y = cb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case pa == "" && pb != "":
		return 1
	case pa != "" && pb == "":
		return -1
	}
	return strings.Compare(pa, pb)
}

func loosePieces(v string) ([]int, string) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	pre := ""
	core := v
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		core, pre = v[:i], v[i+1:]
	}
	var nums []int
	for _, part := range strings.Split(core, ".") {
		digits := part
		for k := 0; k < len(part); k++ {
			if !isDigit(part[k]) {
				digits, pre = part[:k], part[k:]+pre
				break
			}
		}
		n, _ := strconv.Atoi(digits)
		nums = append(nums, n)
	}
	return nums, pre
}
