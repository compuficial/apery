// Package suggest finds the known name closest to a misspelled one, for
// "did you mean" hints.
package suggest

// maxDistance is how many single-character edits still count as a typo.
const maxDistance = 2

// Closest returns the candidate within two edits of name, preferring the
// first of equally close ones, or "" if none is that close.
//
// Example: Closest("mni", []string{"min", "max"}) == "min".
func Closest(name string, candidates []string) string {
	best, bestDist := "", maxDistance+1
	for _, c := range candidates {
		if d := editDistance(name, c); d < bestDist {
			best, bestDist = c, d
		}
	}
	return best
}

// editDistance is the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
