package search

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type FuzzyPolicySuite struct {
	suite.Suite
}

func TestFuzzyPolicySuite(t *testing.T) {
	suite.Run(t, new(FuzzyPolicySuite))
}

func (s *FuzzyPolicySuite) ids(index *Index, query string) []string {
	return recordIDs(index.Search(context.Background(), query))
}

func (s *FuzzyPolicySuite) TestExactNamePrefixAndSubsequenceRanking() {
	records := []Record{
		{ID: "meta-popular", Title: "Popular", Fields: []Field{{Kind: FieldKeyword, Value: "firefox"}}},
		{ID: "fuzzy-popular", Fields: []Field{{Kind: FieldName, Value: "FiRareFuelOtherX"}}},
		{ID: "exact", Title: "Firefox"},
	}
	index := NewIndex(records, DefaultPolicy())
	got := index.SearchWithUsage(context.Background(), "firefox", map[string]int64{
		"meta-popular": 99, "fuzzy-popular": 1000,
	})
	s.Equal([]string{"exact", "meta-popular", "fuzzy-popular"}, recordIDs(got))

	quality := NewIndex([]Record{
		{ID: "substring", Title: "myfirefox"},
		{ID: "prefix", Title: "firefox nightly"},
	}, DefaultPolicy())
	s.Equal("prefix", s.ids(quality, "fire")[0])
	s.Equal("substring", s.ids(quality, "fox")[0])
}

func (s *FuzzyPolicySuite) TestSubsequenceAndBoundaryQuality() {
	index := NewIndex([]Record{
		{ID: "fuzzy", Title: "FiRareFuelOtherX"},
		{ID: "firefox", Title: "Firefox"},
		{ID: "unrelated", Title: "Calculator"},
	}, DefaultPolicy())
	s.ElementsMatch([]string{"fuzzy", "firefox"}, s.ids(index, "ffx"))

	boundaryIndex := NewIndex([]Record{
		{ID: "dense", Title: "fastfirefox"},
		{ID: "boundary", Title: "Fast Firefox"},
		{ID: "camel", Title: "FastFirefox"},
	}, DefaultPolicy())
	got := s.ids(boundaryIndex, "ff")
	s.Require().Len(got, 3)
	s.ElementsMatch([]string{"boundary", "camel"}, got[:2])
	s.Equal("dense", got[2])
}

func (s *FuzzyPolicySuite) TestTokensRequireSeparateFieldValuesAndAllMustMatch() {
	index := NewIndex([]Record{
		{ID: "separate", Fields: []Field{{Kind: FieldName, Value: "red"}, {Kind: FieldKeyword, Value: "fox"}}},
		{ID: "joined-only", Fields: []Field{{Kind: FieldName, Value: "redfox"}}},
		{ID: "missing-token", Fields: []Field{{Kind: FieldName, Value: "red"}}},
	}, DefaultPolicy())
	s.Equal([]string{"separate", "joined-only"}, s.ids(index, "red fox"))
	s.NotContains(s.ids(index, "red fox"), "missing-token")
	s.Equal([]string{"joined-only"}, s.ids(index, "redfox"))
}

func (s *FuzzyPolicySuite) TestBaseAndLocalizedNamesAndUnicodeNormalization() {
	index := NewIndex([]Record{
		{
			ID: "base", Title: "Firefox",
			Fields: []Field{{Kind: FieldName, Value: "Firefox"}, {Kind: FieldName, Value: "Файрфокс"}},
		},
		{ID: "cyrillic", Title: "Браузер", Fields: []Field{{Kind: FieldName, Value: "Мозила"}}},
		{ID: "composed", Title: "Café"},
	}, DefaultPolicy())
	s.Equal("base", s.ids(index, "файрфокс")[0])
	s.Equal("cyrillic", s.ids(index, "мзл")[0])
	s.Equal("composed", s.ids(index, "cafe\u0301")[0])
}

func (s *FuzzyPolicySuite) TestUsageTieBreakAndRankingBeforeCustomLimit() {
	records := []Record{
		{ID: "popular", Title: "Firefox", ProviderID: "provider"},
		{ID: "quiet", Title: "Firefox", ProviderID: "provider"},
		{ID: "metadata", Title: "Other", Fields: []Field{{Kind: FieldKeyword, Value: "Firefox"}}},
	}
	index := NewIndex(records, DefaultPolicy())
	got := index.SearchWithUsage(context.Background(), "firefox", map[string]int64{"popular": 99})
	s.Equal([]string{"popular", "quiet", "metadata"}, recordIDs(got))

	policy := DefaultPolicy()
	policy.ResultLimit = 1
	limitedRecords := []Record{
		{ID: "weaker", Title: "Firefox Browser"},
		{ID: "winner", Title: "Firefox"},
	}
	limited := NewIndex(limitedRecords, policy)
	policy.ResultLimit = 2
	s.Equal([]string{"winner"}, recordIDs(limited.SearchWithUsage(context.Background(), "firefox", nil)))
}

func (s *FuzzyPolicySuite) TestPolicyFieldPriorityAndValueCopy() {
	records := []Record{
		{ID: "name", Title: "Orchid"},
		{ID: "keyword", Title: "Other", Fields: []Field{{Kind: FieldKeyword, Value: "orchid"}}},
	}
	policy := DefaultPolicy()
	policy.ExactNameFirst = false
	policy.FieldOrder = [6]FieldKind{FieldKeyword, FieldName, FieldGenericName, FieldCategory, FieldComment, FieldID}
	index := NewIndex(records, policy)
	policy.FieldOrder = [6]FieldKind{FieldName, FieldGenericName, FieldKeyword, FieldCategory, FieldComment, FieldID}
	policy.ExactNameFirst = true
	s.Equal("keyword", s.ids(index, "orchid")[0])
}

func (s *FuzzyPolicySuite) TestPolicyMatchOrderIsTunable() {
	records := []Record{
		{ID: "exact", Title: "fox"},
		{ID: "prefix", Title: "foxhole"},
	}
	policy := DefaultPolicy()
	policy.ExactNameFirst = false
	policy.MatchOrder = [5]MatchKind{MatchPrefix, MatchExact, MatchSubstring, MatchSubsequence, MatchTypo}
	index := NewIndex(records, policy)
	s.Equal("prefix", s.ids(index, "fox")[0])
}
