package search

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
)

type TypoSuite struct {
	suite.Suite
}

func TestTypoSuite(t *testing.T) {
	suite.Run(t, new(TypoSuite))
}

func (s *TypoSuite) TestEditsUnicodeAndBoundaries() {
	cases := []struct {
		name, word, query string
		matches           bool
	}{
		{"transposition", "Firefox", "firefxo", true},
		{"insertion", "Firefox", "firefoxx", true},
		{"deletion", "Firefox", "firefx", true},
		{"substitution", "Firefox", "firefax", true},
		{"three runes", "abc", "axc", false},
		{"four runes", "abcd", "abxd", true},
		{"seven one edit", "abcdefg", "abcdexg", true},
		{"seven two edits", "abcdefg", "abxdexg", false},
		{"eight two edits", "abcdefgh", "abxdexgh", true},
		{"eight three edits", "abcdefgh", "abxdexxh", false},
		{"Cyrillic transposition", "Браузер", "браузре", true},
		{"Cyrillic three runes", "кот", "кит", false},
		{"Cyrillic four runes", "крот", "крат", true},
		{"canonical Unicode", "Caféteria", "cafe\u0301terxa", true},
		{"unrelated", "Firefox", "zxqwvbn", false},
		{"whole word only", "Firefox", "firz", false},
		{"word length pruning", "abcd", "abxcd", true},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			index := NewIndex([]Record{
				{ID: "target", Fields: []Field{{Kind: FieldName, Value: tc.word}}},
			}, DefaultPolicy())
			got := recordIDs(index.Search(context.Background(), tc.query))
			if tc.matches {
				s.Equal([]string{"target"}, got)
			} else {
				s.Empty(got)
			}
		})
	}
}

func (s *TypoSuite) TestMixedTokensWordBoundariesAndLongMetadata() {
	index := NewIndex([]Record{
		{ID: "mixed", Fields: []Field{
			{Kind: FieldName, Value: "Firefox"}, {Kind: FieldKeyword, Value: "private browser"},
		}},
		{ID: "missing", Fields: []Field{{Kind: FieldName, Value: "Firefox"}}},
		{ID: "split", Fields: []Field{{Kind: FieldName, Value: "fire"}, {Kind: FieldName, Value: "fox"}}},
		{ID: "long", Fields: []Field{{Kind: FieldComment, Value: strings.Repeat("unrelated ", 2048) + "Firefox"}}},
	}, DefaultPolicy())
	s.Equal([]string{"mixed"}, recordIDs(index.Search(context.Background(), "private firefxo")))
	s.ElementsMatch([]string{"mixed", "missing", "long"}, recordIDs(index.Search(context.Background(), "firefxo")))
	s.Empty(index.Search(context.Background(), "private firefxo absent"))
}

func (s *TypoSuite) TestPolicyOverridesAndSnapshot() {
	records := []Record{{ID: "target", Fields: []Field{{Kind: FieldName, Value: "abcd"}}}}
	policy := DefaultPolicy()
	policy.Typo.Thresholds = [2]TypoThreshold{{MinRunes: 3, MaxEdits: 2}, {MinRunes: 4, MaxEdits: 2}}
	index := NewIndex(records, policy)
	policy.Typo.Enabled = false
	s.Equal([]string{"target"}, recordIDs(index.Search(context.Background(), "abx")))
	s.Equal([]string{"target"}, recordIDs(index.Search(context.Background(), "axyd")))
	s.Empty(NewIndex(records, policy).Search(context.Background(), "axyd"))
	policy.Typo.Enabled = true
	policy.Typo.Thresholds = [2]TypoThreshold{{MinRunes: 5, MaxEdits: 1}, {MinRunes: 8, MaxEdits: 2}}
	s.Empty(NewIndex(records, policy).Search(context.Background(), "abxd"))
}

func (s *TypoSuite) TestOrdinaryDominanceAndStableUsageTiesBeforeLimit() {
	records := []Record{
		{ID: "popular-typo", Title: "Firefox"},
		{ID: "ordinary", Title: "Other", Fields: []Field{{Kind: FieldComment, Value: "firefxo"}}},
		{ID: "subsequence", Title: "Other", Fields: []Field{{Kind: FieldComment, Value: "f-i-r-e-f-x-o"}}},
		{ID: "a", Title: "Firefox"},
		{ID: "z", Title: "Firefox"},
	}
	counts := map[string]int64{"popular-typo": 1000, "z": 1, "a": 1}
	policy := DefaultPolicy()
	index := NewIndex(records, policy)
	for range 3 {
		s.Equal([]string{"ordinary", "subsequence", "popular-typo", "a", "z"},
			recordIDs(index.SearchWithUsage(context.Background(), "firefxo", counts)))
	}
	policy.ResultLimit = 1
	s.Equal([]string{"ordinary"}, recordIDs(NewIndex(records, policy).SearchWithUsage(
		context.Background(), "firefxo", counts)))
}

func (s *TypoSuite) TestTypoFieldThenDistanceQuality() {
	index := NewIndex([]Record{
		{ID: "two", Fields: []Field{{Kind: FieldName, Value: "abcdefgh"}}},
		{ID: "one", Fields: []Field{{Kind: FieldName, Value: "abxdxfzh"}}},
		{ID: "keyword", Fields: []Field{{Kind: FieldKeyword, Value: "abxdxfzh"}}},
	}, DefaultPolicy())
	s.Equal([]string{"one", "two", "keyword"}, recordIDs(index.SearchWithUsage(
		context.Background(), "abxdxfgh", map[string]int64{"two": 1000, "keyword": 2000})))
}
