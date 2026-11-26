package query

import (
	"strings"
	"unicode"
)

type tokenKind int

const (
	tokEOF    tokenKind = iota
	tokLParen           // "("
	tokRParen           // ")"
	tokNot              // "not" or a leading "-"
	tokAnd              // "and"
	tokOr               // "or"
	tokWord             // a term or a bare word
	tokError            // an unterminated quote
)

// token is one lexical token. For tokWord, text is the token without its
// quote marks, and quoted marks each byte of text that was inside quotes, so
// the parser can tell `name:"a b"` from `name:a b`.
type token struct {
	kind   tokenKind
	text   string
	quoted []bool
	// anyQuoted is true when the word had a quote mark.
	anyQuoted bool
	pos       Span
}

// lex splits the query into tokens.
func lex(src string) []token {
	var out []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case isSpace(c):
			i++
		case c == '(':
			out = append(out, token{kind: tokLParen, text: "(", pos: Span{i, i + 1}})
			i++
		case c == ')':
			out = append(out, token{kind: tokRParen, text: ")", pos: Span{i, i + 1}})
			i++
		case c == '-' && i+1 < len(src) && !isSpace(src[i+1]) && src[i+1] != ')':
			out = append(out, token{kind: tokNot, text: "-", pos: Span{i, i + 1}})
			i++
		default:
			t, next := lexWord(src, i)
			out = append(out, t)
			i = next
		}
	}
	return append(out, token{kind: tokEOF, pos: Span{len(src), len(src)}})
}

// lexWord reads one word from start. A word ends at a space or a parenthesis
// outside quotes.
func lexWord(src string, start int) (token, int) {
	var b strings.Builder
	var quoted []bool
	i := start
	inQuote, anyQuoted := false, false
	for i < len(src) {
		c := src[i]
		if c == '"' {
			inQuote = !inQuote
			anyQuoted = true
			i++
			continue
		}
		if !inQuote && (isSpace(c) || c == '(' || c == ')') {
			break
		}
		b.WriteByte(c)
		quoted = append(quoted, inQuote)
		i++
	}
	pos := Span{start, i}
	if inQuote {
		return token{kind: tokError, text: src[start:i], pos: pos}, i
	}
	text := b.String()
	if !anyQuoted {
		switch strings.ToLower(text) {
		case "and":
			return token{kind: tokAnd, text: text, pos: pos}, i
		case "or":
			return token{kind: tokOr, text: text, pos: pos}, i
		case "not":
			return token{kind: tokNot, text: text, pos: pos}, i
		}
	}
	return token{kind: tokWord, text: text, quoted: quoted, anyQuoted: anyQuoted, pos: pos}, i
}

func isSpace(c byte) bool {
	return unicode.IsSpace(rune(c))
}
