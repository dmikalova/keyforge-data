package query

// Fuzzy scoring weights. A match gets points for each character, and more for
// a character that continues a run or starts a word.
const (
	scoreChar      = 1
	scoreRun       = 4
	scoreWordStart = 6
	scoreFirst     = 4
)

// FuzzyScore reports whether the characters of pattern appear in text in
// order, and how good the match is. Both are compared in normalized form, so
// case, diacritics, spaces, and punctuation do not matter. A higher score is
// a better match: runs of adjacent characters and characters at the start of
// a word score more. An empty pattern matches with a score of zero.
func FuzzyScore(pattern, text string) (int, bool) {
	p := Normalize(pattern)
	if p == "" {
		return 0, true
	}
	chars, starts := wordChars(text)
	best, found := 0, false
	// Try each start of the first character, then match the rest greedily.
	for i := range chars {
		if chars[i] != p[0] {
			continue
		}
		score, ok := scoreFrom(p, chars, starts, i)
		if ok && (!found || score > best) {
			best, found = score, true
		}
	}
	return best, found
}

func scoreFrom(p string, chars []byte, starts []bool, first int) (int, bool) {
	score := 0
	if first == 0 {
		score += scoreFirst
	}
	prev := -2
	j := first
	for k := range len(p) {
		for j < len(chars) && chars[j] != p[k] {
			j++
		}
		if j == len(chars) {
			return 0, false
		}
		score += scoreChar
		if j == prev+1 {
			score += scoreRun
		}
		if starts[j] {
			score += scoreWordStart
		}
		prev = j
		j++
	}
	return score, true
}

// wordChars returns the normalized characters of text and marks the ones
// that start a word.
func wordChars(text string) ([]byte, []bool) {
	f := Fold(text)
	var chars []byte
	var starts []bool
	newWord := true
	for i := range len(f) {
		c := f[i]
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			chars = append(chars, c)
			starts = append(starts, newWord)
			newWord = false
			continue
		}
		newWord = true
	}
	return chars, starts
}
