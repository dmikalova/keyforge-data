package query

// Kind is the value type of a field.
type Kind int

// The field kinds.
const (
	// KindText matches a substring of the folded text.
	KindText Kind = iota
	// KindFuzzy matches a fuzzy subsequence, and ranks by match quality.
	KindFuzzy
	// KindEnum matches one of a fixed set of values exactly.
	KindEnum
	// KindNumber compares a number. A number can be X.
	KindNumber
	// KindKeyed matches a key, and can compare the key's number:
	// "keyword:assault>=2".
	KindKeyed
	// KindFlag is the "is:" field. Its values are flag names.
	KindFlag
	// KindDirective changes the display and does not filter.
	KindDirective
)

// FieldInfo describes a field to the parser.
type FieldInfo struct {
	// Name is the full name.
	Name string
	// Alias is the one short name. It can be empty.
	Alias string
	// Kind is the value type.
	Kind Kind
	// Values are the allowed values of an enum, flag, or directive field,
	// and the keys of a keyed field. Empty means any value.
	Values []string
	// Bare is true for a text field that a bare word searches.
	Bare bool
}

// Schema names the fields of a query language.
type Schema interface {
	// Field returns the field with the name or alias.
	Field(name string) (FieldInfo, bool)
	// Fields returns every field.
	Fields() []FieldInfo
}
