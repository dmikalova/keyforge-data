package query

import "strings"

// Fuzzy scoring weights. A match gets points for each character, and more for
// a character that continues a run or starts a word.
const (
	scoreChar      = 1
	scoreRun       = 4
	scoreWordStart = 6
	scoreFirst     = 4
)

// The match tiers, from the worst to the best. A match in a better tier
// always scores more than a match in a worse tier.
const (
	tierScattered = iota
	tierSubstring
	tierWordStart
	tierPrefix
	tierExact
)

// tierWeight is the score of one tier. The points inside a tier stay below
// it.
const tierWeight = 1000

// FuzzyScore reports whether the characters of pattern appear in text in
// order, and how good the match is. Both are compared in normalized form, so
// case, diacritics, spaces, and punctuation do not matter. An empty pattern
// matches with a score of zero.
//
// A higher score is a better match. The tiers, from the best: the whole
// text, a prefix, a contiguous match at the start of a word, a contiguous
// match, and characters scattered in order. Inside a tier, runs of adjacent
// characters and characters at the start of a word score more.
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
	if !found {
		return 0, false
	}
	return tier(p, chars, starts)*tierWeight + min(best, tierWeight-1), true
}

// tier returns the match tier of the normalized pattern in the characters.
func tier(p string, chars []byte, starts []bool) int {
	s := string(chars)
	switch {
	case s == p:
		return tierExact
	case strings.HasPrefix(s, p):
		return tierPrefix
	}
	found := false
	for i := 0; i+len(p) <= len(s); i++ {
		if s[i:i+len(p)] != p {
			continue
		}
		if starts[i] {
			return tierWordStart
		}
		found = true
	}
	if found {
		return tierSubstring
	}
	return tierScattered
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
