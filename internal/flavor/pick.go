package flavor

import "math/rand/v2"

// Pick returns a random string from pool, or "" if the pool is empty.
// Use this from any caller that wants to render flavor text from one of
// the protected pools defined in this package.
func Pick(pool []string) string {
	if len(pool) == 0 {
		return ""
	}
	return pool[rand.IntN(len(pool))]
}
