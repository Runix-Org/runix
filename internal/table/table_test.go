package table

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type TableSuite struct{ suite.Suite }

func TestTableSuite(t *testing.T) { suite.Run(t, new(TableSuite)) }

func (s *TableSuite) TestNumericOrderingAndStableTies() {
	state := New([]Record{{ID: "b", Value: 10}, {ID: "c", Value: 2}, {ID: "a", Value: 2}, {ID: "d", Value: 100}})
	s.Equal([]string{"a", "b", "c", "d"}, ids(state.Records))
	s.True(state.Sort(1))
	s.Equal([]string{"a", "c", "b", "d"}, ids(state.Records))
	s.True(state.Sort(1))
	s.Equal([]string{"d", "b", "c", "a"}, ids(state.Records))
}

func (s *TableSuite) TestSelectUpdateAndRemove() {
	state := New([]Record{
		{ID: "b", Value: 10, Status: "Active"},
		{ID: "a", Value: 2, Status: "Inactive"},
		{ID: "c", Value: 100},
	})
	s.True(state.Select(0))
	s.Equal("a", state.SelectedID)
	s.True(state.Activate("a"))
	s.True(state.Sort(1))
	s.Equal("a", state.SelectedID)
	s.Equal("a", state.ActivatedID)
	s.True(state.Update("a"))
	s.Equal("a", state.SelectedID)
	s.Equal("Active", state.Records[state.SelectedIndex].Status)
	s.True(state.Remove("a"))
	s.Equal(0, state.SelectedIndex)
	s.Equal("b", state.SelectedID)
	s.True(state.Remove("b"))
	s.True(state.Remove("c"))
	s.Equal(-1, state.SelectedIndex)
	s.Empty(state.SelectedID)
}

func (s *TableSuite) TestUpdatesMoveAcrossNumericTies() {
	state := New([]Record{{ID: "a", Value: 2}, {ID: "b", Value: 2}, {ID: "c", Value: 3}})
	s.Require().True(state.Sort(1))
	s.Require().True(state.Select(0))
	s.Require().True(state.Update("a"))
	s.Equal([]string{"b", "a", "c"}, ids(state.Records))
	s.Equal(1, state.SelectedIndex)
	s.Require().True(state.Update("a"))
	s.Equal([]string{"b", "c", "a"}, ids(state.Records))
	s.Equal(2, state.SelectedIndex)
	s.Require().True(state.Sort(1))
	s.Require().True(state.Update("c"))
	s.Equal([]string{"c", "a", "b"}, ids(state.Records))
	s.Equal("a", state.SelectedID)
	s.Equal(1, state.SelectedIndex)
	s.Require().True(state.Activate("a"))
	s.Equal("a", state.ActivatedID)
	s.Require().True(state.Remove("c"))
	s.Equal(0, state.SelectedIndex)
	s.Equal("a", state.SelectedID)
}

func (s *TableSuite) TestEmptySingleTransitions() {
	state := New(nil)
	s.Equal(-1, state.SelectedIndex)
	state = New([]Record{{ID: "only", Title: "one"}})
	s.Equal(0, state.SelectedIndex)
	s.Equal("only", state.SelectedID)
	s.True(state.Remove("only"))
	s.Empty(state.Records)
	s.Equal(-1, state.SelectedIndex)
}

func ids(records []Record) []string {
	result := make([]string, len(records))
	for i, record := range records {
		result[i] = record.ID
	}
	return result
}
