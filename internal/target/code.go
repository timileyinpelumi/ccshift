package target

import "hash/fnv"

// Consonants only, so a code never spells a word that could also be the start of a name.
const codeLetters = "BCDFGHJKLMNPQRSTVWXZ"

// Code is a short handle for a session, derived from its id so it survives restarts.
func Code(id string) string {
	h := fnv.New32a()
	h.Write([]byte(id))
	n := h.Sum32()
	b := make([]byte, 3)
	for i := range b {
		b[i] = codeLetters[n%uint32(len(codeLetters))]
		n /= uint32(len(codeLetters))
	}
	return string(b)
}
