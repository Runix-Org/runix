package search

import (
	"cmp"
	"context"
	"strings"
	"unicode"

	"github.com/sahilm/fuzzy"
)

type quality struct {
	exactName                      bool
	worstKind, sumKinds, sumFields int
	subsequence                    int
}

func compareQuality(a, b quality) int {
	if a.exactName != b.exactName {
		if a.exactName {
			return -1
		}
		return 1
	}
	if order := cmp.Compare(a.worstKind, b.worstKind); order != 0 {
		return order
	}
	if order := cmp.Compare(a.sumKinds, b.sumKinds); order != 0 {
		return order
	}
	if order := cmp.Compare(a.sumFields, b.sumFields); order != 0 {
		return order
	}
	return cmp.Compare(b.subsequence, a.subsequence)
}

func splitWords(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func lexicalKind(field *indexedField, token string) (MatchKind, bool) {
	if field.normalized == token {
		return MatchExact, true
	}
	prefix := strings.HasPrefix(field.normalized, token)
	for _, word := range field.words {
		if word == token {
			return MatchExact, true
		}
		prefix = prefix || strings.HasPrefix(word, token)
	}
	if prefix {
		return MatchPrefix, true
	}
	if strings.Contains(field.normalized, token) {
		return MatchSubstring, true
	}
	return 0, false
}

type tokenQuality struct {
	kind, field, subsequence int
	matched                  bool
}

func betterToken(a, b tokenQuality) bool {
	if !b.matched {
		return true
	}
	if a.kind != b.kind {
		return a.kind < b.kind
	}
	if a.field != b.field {
		return a.field < b.field
	}
	return a.subsequence > b.subsequence
}

func (i *Index) scoreRecord(ctx context.Context, record *indexedRecord, query string, tokens []string) (quality, bool) {
	if i.policy.ExactNameFirst {
		for _, field := range record.fields {
			if field.kind == FieldName && field.normalized == query {
				return quality{exactName: true}, true
			}
		}
	}
	var total quality
	for _, token := range tokens {
		var best tokenQuality
		for n := range record.fields {
			field := &record.fields[n]
			if ctx.Err() != nil {
				return quality{}, false
			}
			if kind, matched := lexicalKind(field, token); matched {
				value := tokenQuality{kind: i.policy.matchRank(kind), field: field.priority, matched: true}
				if betterToken(value, best) {
					best = value
				}
			}
		}
		fuzzyRank := i.policy.matchRank(MatchSubsequence)
		if !best.matched || fuzzyRank < best.kind {
			// The library reuses scratch storage between independent values.
			// The iterator checks cancellation between fields, without joining or
			// truncating metadata; Match.Index still identifies the original field.
			matches := fuzzy.FindFromIterNoSort(token, func(yield func(string) bool) {
				for _, field := range record.fields {
					if ctx.Err() != nil || !yield(field.original) {
						return
					}
				}
			})
			for _, match := range matches {
				value := tokenQuality{
					kind: fuzzyRank, field: record.fields[match.Index].priority,
					subsequence: match.Score, matched: true,
				}
				if betterToken(value, best) {
					best = value
				}
			}
		}
		if !best.matched {
			return quality{}, false
		}
		total.worstKind = max(total.worstKind, best.kind)
		total.sumKinds += best.kind
		total.sumFields += best.field
		total.subsequence += best.subsequence
	}
	return total, true
}
