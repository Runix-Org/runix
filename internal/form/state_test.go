package form

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"
)

type StateSuite struct{ suite.Suite }

func TestStateSuite(t *testing.T) { suite.Run(t, new(StateSuite)) }

func validValues() Values {
	return Values{Name: "Ada", Notes: "Hello", Password: "correct horse", Choice: "Personal", Accepted: true}
}

func (s *StateSuite) TestValidationAndCorrection() {
	state := New()
	state.Edit(Values{Name: " \t", Notes: " ", Password: "ééééééé", Choice: "Other"})
	_, ok := state.Submit()
	s.False(ok)
	view := state.Snapshot()
	s.False(view.Busy)
	s.Equal("Check the form and try again", view.Status)
	s.Len(view.Errors, 5)
	for _, field := range []string{"accepted", "choice", "name", "notes", "password"} {
		s.Contains(view.Errors, field)
	}

	state.Edit(validValues())
	view = state.Snapshot()
	s.Empty(view.Errors)
	s.Equal("Ready", view.Status)
	_, ok = state.Submit()
	s.True(ok)
}

func (s *StateSuite) TestDuplicatePreventionAndCoherentRevision() {
	state := New()
	state.Edit(validValues())
	ticket, ok := state.Submit()
	s.Require().True(ok)
	_, ok = state.Submit()
	s.False(ok)

	newValues := validValues()
	newValues.Name = "Grace"
	state.Edit(newValues)
	s.Equal(validValues(), ticket.values)
	s.Equal(newValues, state.values)
	state.Complete(ticket)
	view := state.Snapshot()
	s.False(view.Busy)
	s.Equal("Submission completed; newer edits were not submitted", view.Status)
	s.Equal(ticket.Revision, view.SubmittedRevision)
	s.Equal(ticket.Revision+1, view.Revision)
}

func (s *StateSuite) TestCancelResubmitRejectsStaleCompletion() {
	state := New()
	state.Edit(validValues())
	stale, ok := state.Submit()
	s.Require().True(ok)
	state.Cancel()
	state.Cancel()
	fresh, ok := state.Submit()
	s.Require().True(ok)
	s.Equal(stale.Revision, fresh.Revision)
	s.NotEqual(stale, fresh)

	state.Complete(stale)
	s.True(state.Snapshot().Busy)
	s.Equal("Submitting", state.Snapshot().Status)
	state.Complete(fresh)
	s.False(state.Snapshot().Busy)
	s.Equal("Submitted", state.Snapshot().Status)
}

func (s *StateSuite) TestSnapshotJSONNeverExposesSecretsOrValues() {
	state := New()
	values := validValues()
	values.Password = "unique-secret-value"
	values.Name = "private-name"
	state.Edit(values)
	_, ok := state.Submit()
	s.Require().True(ok)
	encoded, err := json.Marshal(state.Snapshot())
	s.Require().NoError(err)
	for _, private := range []string{values.Password, values.Name, values.Notes} {
		s.NotContains(string(encoded), private)
	}
}
