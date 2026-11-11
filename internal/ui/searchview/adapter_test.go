package searchview

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"runix/internal/search"
)

type ActivationSuite struct{ suite.Suite }

func TestActivationSuite(t *testing.T) { suite.Run(t, new(ActivationSuite)) }

func (s *ActivationSuite) TestExactClickedIdentityAndBusyRejection() {
	var calls []search.Record
	a := &Adapter{
		value: snapshot{
			Generation: 4, AppliedGeneration: 4, SelectedIndex: 0,
			SelectedID: "first", ActivationEnabled: true,
		},
		records: []search.Record{{ID: "first", CatalogGeneration: 10}, {ID: "clicked", CatalogGeneration: 20}},
	}
	selected := ""
	a.selected = func(id string) { selected = id }
	a.activate = func(id string, generation uint64) {
		s.Equal(id, selected, "activation must notify selection before submitting the exact clicked item")
		calls = append(calls, search.Record{ID: id, CatalogGeneration: generation})
		a.value.ActivationEnabled = false
	}
	s.True(a.activateTarget("clicked", 4))
	s.False(a.activateTarget("clicked", 4))
	s.Equal([]search.Record{{ID: "clicked", CatalogGeneration: 20}}, calls)
}

func (s *ActivationSuite) TestStalePressNeverActivatesReplacement() {
	for _, scenario := range []string{"replaced query", "loading", "removed identity", "missing identity"} {
		s.Run(scenario, func() {
			a := &Adapter{
				value:   snapshot{Generation: 5, AppliedGeneration: 5, ActivationEnabled: true},
				records: []search.Record{{ID: "replacement", CatalogGeneration: 30}},
			}
			calls := 0
			a.activate = func(string, uint64) { calls++ }
			id, generation := "replacement", uint64(5)
			switch scenario {
			case "replaced query":
				generation = 4
			case "loading":
				a.value.Loading = true
			case "removed identity":
				id = "removed"
			case "missing identity":
				id = ""
			}
			s.False(a.activateTarget(id, generation))
			s.Zero(calls)
		})
	}
}
