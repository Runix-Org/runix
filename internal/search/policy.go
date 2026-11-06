package search

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// MatchKind identifies textual quality independently of provider metadata.
type MatchKind uint8

const (
	MatchExact MatchKind = iota
	MatchPrefix
	MatchSubstring
	MatchSubsequence
	MatchTypo
)

// TypoThreshold assigns an edit budget starting at MinRunes normalized code points.
type TypoThreshold struct {
	MinRunes, MaxEdits int
}

// TypoPolicy controls whole-word OSA edit matching independently of subsequences.
type TypoPolicy struct {
	Enabled    bool
	Thresholds [2]TypoThreshold
}

// Policy is copied into an immutable index. Normalize must be a pure function,
// applying the same case/Unicode rules to both indexed values and queries.
// FieldOrder and MatchOrder list each supported kind once, best first.
// Replace the index to change policy; never mutate a search in progress.
type Policy struct {
	ResultLimit    int
	Normalize      func(string) string
	FieldOrder     [6]FieldKind
	MatchOrder     [5]MatchKind
	ExactNameFirst bool
	Typo           TypoPolicy
}

// DefaultPolicy returns independently owned product tuning, without global knobs.
func DefaultPolicy() Policy {
	return Policy{
		ResultLimit: 200,
		Normalize:   normalize,
		FieldOrder: [6]FieldKind{
			FieldName, FieldGenericName, FieldKeyword, FieldID, FieldCategory, FieldComment,
		},
		MatchOrder:     [5]MatchKind{MatchExact, MatchPrefix, MatchSubstring, MatchSubsequence, MatchTypo},
		ExactNameFirst: true,
		Typo: TypoPolicy{Enabled: true, Thresholds: [2]TypoThreshold{
			{MinRunes: 4, MaxEdits: 1}, {MinRunes: 8, MaxEdits: 2},
		}},
	}
}

func normalize(value string) string {
	return norm.NFC.String(strings.ToLower(originalValue(value)))
}

func originalValue(value string) string {
	return norm.NFC.String(strings.Join(strings.Fields(value), " "))
}

func (p Policy) fieldRank(kind FieldKind) int {
	for rank, entry := range p.FieldOrder {
		if entry == kind {
			return rank
		}
	}
	return len(p.FieldOrder)
}

func (p Policy) matchRank(kind MatchKind) int {
	for rank, entry := range p.MatchOrder {
		if entry == kind {
			return rank
		}
	}
	return len(p.MatchOrder)
}

func (p TypoPolicy) budget(runes int) int {
	if !p.Enabled {
		return 0
	}
	budget, threshold := 0, -1
	for _, entry := range p.Thresholds {
		if runes >= entry.MinRunes && entry.MinRunes >= threshold {
			budget, threshold = entry.MaxEdits, entry.MinRunes
		}
	}
	return budget
}
