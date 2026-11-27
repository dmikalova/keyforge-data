// Package derive computes card fields that Master Vault does not send: the
// keywords and the Enhance icons. It reads the card text as Master Vault
// sends it, with its icon glyphs and markup.
package derive

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Keyword is a rules keyword. Some keywords take a number: "Assault 2".
type Keyword struct {
	Name string
	// Valued is true for a keyword that takes a number.
	Valued bool
}

// Keywords lists the KeyForge keywords.
var Keywords = []Keyword{
	{Name: "alpha"},
	{Name: "assault", Valued: true},
	{Name: "deploy"},
	{Name: "elusive"},
	{Name: "hazardous", Valued: true},
	{Name: "invulnerable"},
	{Name: "omega"},
	{Name: "omni"},
	{Name: "poison"},
	{Name: "skirmish"},
	{Name: "splash-attack", Valued: true},
	{Name: "taunt"},
	{Name: "treachery"},
	{Name: "versatile"},
}

// KeywordNames returns the keyword names.
func KeywordNames() []string {
	out := make([]string, len(Keywords))
	for i, k := range Keywords {
		out[i] = k.Name
	}
	return out
}

// Icon names for Enhance and bonus icons.
const (
	IconAember  = "aember"
	IconCapture = "capture"
	IconDamage  = "damage"
	IconDraw    = "draw"
	IconDiscard = "discard"
	IconTide    = "tide"
)

// glyphs maps the icon-font glyphs in Master Vault card text to icon names.
// House enhancement icons map to the house name.
var glyphs = map[rune]string{
	0xF360: IconAember,
	0xF361: IconDamage,
	0xF36E: IconDraw,
	0xF565: IconCapture,
	0xF372: IconDiscard,
	0xF566: IconTide,
	0xF379: "brobnar",
	0xF37A: "dis",
	0xF37B: "ekwidon",
	0xF37C: "geistoid",
	0xF37D: "logos",
	0xF37E: "mars",
	0xF37F: "skyborn",
	0xF386: "redemption",
	0xF387: "sanctum",
	0xF388: "saurian",
	0xF389: "shadows",
	0xF38A: "staralliance",
	0xF38B: "untamed",
	0xF390: "unfathomable",
	0xF391: "ouboros",
}

// captureFirst and captureSecond are the two glyphs of the capture pair.
const (
	captureFirst  = 0xF36F
	captureSecond = 0xF560
)

// markup maps the <X> tokens and the bare letters of Enhance to icon names.
var markup = map[string]string{
	"A": IconAember,
	"C": IconCapture,
	"D": IconDamage,
	"R": IconDraw,
}

// sentenceRe splits a line into sentences at a period.
var sentenceRe = regexp.MustCompile(`[^.]+\.?`)

// reminderRe matches a parenthetical reminder.
var reminderRe = regexp.MustCompile(`\([^)]*\)`)

// Text returns the card text with line breaks as newlines and without
// reminder text.
func Text(s string) string {
	s = strings.NewReplacer("\u000b", "\n", "\r\n", "\n", "\r", "\n").Replace(s)
	return reminderRe.ReplaceAllString(s, "")
}

// CardKeywords returns the keywords that start the card text, with their
// numbers (zero for a keyword without one). Keywords in the middle of an
// ability ("gains elusive") do not count. The keyword lines end at the
// first sentence that is not a keyword.
func CardKeywords(text string) map[string]int {
	out := map[string]int{}
	for line := range strings.SplitSeq(Text(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, s := range sentenceRe.FindAllString(line, -1) {
			s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "."))
			if s == "" {
				continue
			}
			for part := range strings.SplitSeq(s, ",") {
				name, n, ok := parseKeyword(strings.TrimSpace(part))
				if !ok {
					return out
				}
				out[name] += n
			}
		}
	}
	return out
}

// parseKeyword reads one keyword with an optional number.
func parseKeyword(s string) (string, int, bool) {
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	if len(fields) == 0 || len(fields) > 2 {
		return "", 0, false
	}
	i := slices.IndexFunc(Keywords, func(k Keyword) bool { return k.Name == fields[0] })
	if i < 0 {
		return "", 0, false
	}
	k := Keywords[i]
	if !k.Valued {
		return k.Name, 0, len(fields) == 1
	}
	if len(fields) != 2 {
		return "", 0, false
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, false
	}
	return k.Name, n, true
}

// enhanceRe finds the icons after "Enhance" up to the end of the sentence.
var enhanceRe = regexp.MustCompile(`(?i)\benhance\b([^.\n]*)`)

// Enhance returns the number of icons of each kind that the card's Enhance
// keywords give. It returns nil for a card without Enhance.
func Enhance(text string) map[string]int {
	text = Text(text)
	var out map[string]int
	for _, m := range enhanceRe.FindAllStringSubmatch(text, -1) {
		icons := enhanceIcons(m[1])
		if len(icons) == 0 {
			continue
		}
		if out == nil {
			out = map[string]int{}
		}
		for _, ic := range icons {
			out[ic]++
		}
	}
	return out
}

// enhanceIcons reads the icons in the text after "Enhance": glyphs, <X>
// tokens, or a run of bare letters ("AAD", with "PT" for capture).
func enhanceIcons(s string) []string {
	var out []string
	rs := []rune(strings.TrimSpace(s))
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == captureFirst && i+1 < len(rs) && rs[i+1] == captureSecond:
			out = append(out, IconCapture)
			i++
		case glyphs[r] != "":
			out = append(out, glyphs[r])
		case r == '<':
			j := slices.Index(rs[i:], '>')
			if j < 0 {
				return out
			}
			if ic, ok := markup[string(rs[i+1:i+j])]; ok {
				out = append(out, ic)
			}
			i += j
		case r == 'P' && i+1 < len(rs) && rs[i+1] == 'T':
			out = append(out, IconCapture)
			i++
		case markup[string(r)] != "" && isLetterRun(rs):
			out = append(out, markup[string(r)])
		case r == ' ' || r == ',':
		default:
			// Words such as "Enhance cards" in rules text are not icons.
			return out
		}
	}
	return out
}

// isLetterRun reports whether the text is a run of icon letters, such as
// "AAD" or "APT", and not a word.
func isLetterRun(rs []rune) bool {
	for _, r := range rs {
		if r != ' ' && !strings.ContainsRune("ACDRPT", r) {
			return false
		}
	}
	return true
}

// Abilities are the ability triggers, in the order that people read them:
// Play first, then Fight and Reap. "After Fight" is the same trigger as
// "Fight", and "After Reap" is the same as "Reap".
var Abilities = []string{
	"play", "fight", "reap", "beforefight", "action", "omni", "destroyed", "leavesplay", "scrap",
}

// abilityRe finds a trigger list at the start of a line, a sentence, or a
// quote: "Play/After Fight/After Reap:". A trigger in quotes ("gains,
// “Reap: ...”") is an ability that the card gives, and it counts, because a
// search for the trigger must find the card.
var abilityRe = regexp.MustCompile(
	`(?m)(?:^|[.!]\s+|[“"])((?:(?:After |Before )?(?:Play|Fight|Reap|Action|Omni|Destroyed|Leaves Play|Scrap)/?)+):`,
)

// CardAbilities returns the card's ability triggers in the order of
// Abilities, or nil.
func CardAbilities(text string) []string {
	seen := map[string]bool{}
	for _, m := range abilityRe.FindAllStringSubmatch(Text(text), -1) {
		for t := range strings.SplitSeq(m[1], "/") {
			t = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "After "))
			seen[strings.ReplaceAll(t, " ", "")] = true
		}
	}
	var out []string
	for _, a := range Abilities {
		if seen[a] {
			out = append(out, a)
		}
	}
	return out
}

// TideToken stands for the tide icon in Display text. The site replaces it
// with the icon.
const TideToken = "{tide}"

// iconWords are the words for the icons in card text.
var iconWords = map[string]string{
	IconAember: "Æmber", IconDamage: "damage", IconDraw: "draw", IconCapture: "capture",
	IconDiscard: "discard",
}

// amountRe finds an amount with an icon: "1A", "2<D>", "X<A>".
var amountRe = regexp.MustCompile(`\b(\d+|X)[ \x{a0}]?(?:<([AD])>|([AD]))(\W|$)`)

// tokenRe finds an icon token without an amount: "<A>".
var tokenRe = regexp.MustCompile(`<([ACDR])>`)

// Display returns the card text for people: line breaks as newlines, and
// icons as words. "Gain 1A" becomes "Gain 1 Æmber", "Deal 2<D>" becomes
// "Deal 2 damage", and the icon-font glyphs become their names. Keywords and
// Enhance read the text as Master Vault sends it, not this text.
func Display(s string) string {
	s = Clean(s)
	s = amountRe.ReplaceAllStringFunc(s, func(m string) string {
		p := amountRe.FindStringSubmatch(m)
		icon := cmp.Or(p[2], p[3])
		return p[1] + " " + iconWords[markup[icon]] + p[4]
	})
	s = tokenRe.ReplaceAllStringFunc(s, func(m string) string {
		return iconWords[markup[m[1:2]]]
	})
	var b strings.Builder
	rs := []rune(s)
	prevIcon := false
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		word := ""
		switch {
		case r == captureFirst && i+1 < len(rs) && rs[i+1] == captureSecond:
			word = iconWords[IconCapture]
			i++
		case glyphs[r] == IconTide:
			// The tide icon has no word. The site draws it from the token.
			word = TideToken
		case glyphs[r] != "":
			word = cmp.Or(iconWords[glyphs[r]], houseWord(glyphs[r]))
		}
		if word == "" {
			b.WriteRune(r)
			prevIcon = false
			continue
		}
		if prevIcon || endsWithDigit(&b) {
			b.WriteByte(' ')
		}
		b.WriteString(word)
		prevIcon = true
	}
	return bareAemberRe.ReplaceAllString(b.String(), "${1} "+iconWords[IconAember]+"${2}")
}

// bareAemberRe finds an old printing's "A" for Æmber in the middle of a
// sentence: "their remaining A.", "each creature with A on it". The "A"
// follows a lowercase word, and punctuation or a word such as "on" or
// "than" follows it. A capital "A" at the start of a sentence is the
// article: "Action: A friendly creature".
var bareAemberRe = regexp.MustCompile(
	`(\b(?:[a-z]+|Your|Their|Each|All|Any|No|The)) A([.,;:]|\s+(?:on|from|in|cannot|than|equal|spent|to|and|or|you|that|they|would|is|are|lost|stolen|bonus|by)\b|\s+\()`,
)

// houseWord returns a house icon name as a word: "staralliance" becomes
// "Star Alliance".
func houseWord(name string) string {
	if name == "staralliance" {
		return "Star Alliance"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// cleaner turns the Master Vault markup and the typesetting characters into
// plain text.
var cleaner = strings.NewReplacer(
	"\r\n", "\n", "\r", "\n", "\u000b", "\n",
	// A soft return is a line break inside a paragraph.
	"<softreturn>", "\n",
	// A non-breaking space keeps two words on one line: "Squire Alys".
	"<nonbreak>", "\u00a0",
	// The narrow no-break space is used as a normal space.
	"\u202f", " ",
	// The non-breaking hyphen is a normal hyphen for search.
	"\u2011", "-",
	// Master Vault ends some texts with zero-width characters. They are not
	// visible and make the same text of two printings differ.
	"\ufeff", "", "\u200b", "", "\u200c", "", "\u200d", "", "\u2060", "",
)

// spacesRe finds runs of spaces and the spaces next to a line break.
var spacesRe = regexp.MustCompile(` *\n *| {2,}`)

// Clean returns text or flavor text without the Master Vault markup:
// <softreturn> becomes a line break, <nonbreak> a non-breaking space, and
// the typesetting spaces and hyphens become plain ones. It also removes the
// spaces at the ends of lines and runs of spaces.
func Clean(s string) string {
	s = cleaner.Replace(s)
	s = spacesRe.ReplaceAllStringFunc(s, func(m string) string {
		if strings.Contains(m, "\n") {
			return "\n"
		}
		return " "
	})
	return strings.TrimSpace(s)
}

// attributionRe finds the dash of a flavor text attribution after the
// closing quote, with any dash and spacing: "” - Name", "” –Name".
var attributionRe = regexp.MustCompile(`([”"])\s*[-–—]+\s*`)

// Flavor returns clean flavor text with one style of attribution: the
// quote, a line break, an em dash, and the name ("”\n—Squire Alys").
func Flavor(s string) string {
	s = closeQuoteRe.ReplaceAllString(Clean(s), "$1”$2")
	s = openQuoteRe.ReplaceAllString(s, "$1“$2")
	return attributionRe.ReplaceAllString(s, "$1\n—")
}

// Master Vault sometimes types a closing mark at the start of a quote and an
// opening mark at its end. A mark after a letter or punctuation that comes
// before a space or the end closes the quote. A mark at the start, or after
// a space or "(", that comes before a letter opens it.
var (
	closeQuoteRe = regexp.MustCompile(`([^\s\x{a0}(])“([\s\x{a0}]|$)`)
	openQuoteRe  = regexp.MustCompile(`(^|[\s\x{a0}(])”([^\s\x{a0}”])`)
)

// endsWithDigit reports whether the text so far ends with a digit, so that
// "2" and a damage glyph become "2 damage".
func endsWithDigit(b *strings.Builder) bool {
	s := b.String()
	return s != "" && s[len(s)-1] >= '0' && s[len(s)-1] <= '9'
}
