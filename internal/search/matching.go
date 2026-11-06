package search

import (
	"cmp"
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	edlib "github.com/hbollon/go-edlib"
	"github.com/sahilm/fuzzy"
)

type quality struct {
	exactName                      bool
	typo                           bool
	worstKind, sumKinds, sumFields int
	distance, subsequence          int
}

func compareQuality(a, b quality) int {
	if a.typo != b.typo {
		if a.typo {
			return 1
		}
		return -1
	}
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
	if order := cmp.Compare(a.distance, b.distance); order != 0 {
		return order
	}
	return cmp.Compare(b.subsequence, a.subsequence)
}

func splitWords(value string) []indexedWord {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	words := make([]indexedWord, len(parts))
	for n, part := range parts {
		words[n] = indexedWord{text: part, runes: utf8.RuneCountInString(part)}
	}
	return words
}

func lexicalKind(field *indexedField, token string) (MatchKind, bool) {
	if field.normalized == token {
		return MatchExact, true
	}
	prefix := strings.HasPrefix(field.normalized, token)
	for _, word := range field.words {
		if word.text == token {
			return MatchExact, true
		}
		prefix = prefix || strings.HasPrefix(word.text, token)
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
	kind, field, distance, subsequence int
	matched                            bool
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
	if a.distance != b.distance {
		return a.distance < b.distance
	}
	return a.subsequence > b.subsequence
}

func (i *Index) scoreRecord(
	ctx context.Context, record *indexedRecord, query string, tokens []queryToken,
) (quality, bool) {
	if i.policy.ExactNameFirst {
		for _, field := range record.fields {
			if field.kind == FieldName && field.normalized == query {
				return quality{exactName: true}, true
			}
		}
	}
	var total quality
	for n := range tokens {
		token := &tokens[n]
		var best tokenQuality
		for n := range record.fields {
			field := &record.fields[n]
			if ctx.Err() != nil {
				return quality{}, false
			}
			if kind, matched := lexicalKind(field, token.text); matched {
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
			matches := fuzzy.FindFromIterNoSort(token.text, func(yield func(string) bool) {
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
		// An ordinary match always wins for this token, even under custom kind ordering.
		if !best.matched && token.edits > 0 {
			best = i.typoMatch(ctx, record, token)
		}
		if !best.matched {
			return quality{}, false
		}
		total.worstKind = max(total.worstKind, best.kind)
		total.sumKinds += best.kind
		total.sumFields += best.field
		total.subsequence += best.subsequence
		total.distance += best.distance
		total.typo = total.typo || best.distance > 0
	}
	return total, true
}

func (i *Index) typoMatch(ctx context.Context, record *indexedRecord, token *queryToken) tokenQuality {
	var best tokenQuality
	for _, field := range record.fields {
		for _, word := range field.words {
			if ctx.Err() != nil {
				return tokenQuality{}
			}
			if word.runes-token.runes > token.edits || token.runes-word.runes > token.edits {
				continue
			}
			distance, known := token.distances[word.text]
			// Query-local memoization reuses distances across repeated metadata words and records.
			if !known {
				distance = edlib.OSADamerauLevenshteinDistance(token.text, word.text)
				if token.distances == nil {
					token.distances = make(map[string]int)
				}
				token.distances[word.text] = distance
			}
			if distance > token.edits {
				continue
			}
			value := tokenQuality{
				kind: i.policy.matchRank(MatchTypo), field: field.priority, distance: distance, matched: true,
			}
			if betterToken(value, best) {
				best = value
			}
		}
	}
	return best
}
