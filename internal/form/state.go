package form

import (
	"strings"
	"unicode/utf8"
)

// Values contains editable form data. Values are never included in Snapshot.
type Values struct {
	Name     string `json:"name"`
	Notes    string `json:"notes"`
	Password string `json:"password"`
	Choice   string `json:"choice"`
	Accepted bool   `json:"accepted"`
}

// Ticket identifies one immutable submission attempt.
type Ticket struct {
	Revision   uint64
	generation uint64
	values     Values
}

// Snapshot contains public form status, never submitted or editable values.
type Snapshot struct {
	Revision          uint64            `json:"revision"`
	SubmittedRevision uint64            `json:"submitted_revision"`
	Busy              bool              `json:"busy"`
	Status            string            `json:"status"`
	Errors            map[string]string `json:"errors"`
}

// State owns form validation and submission state.
type State struct {
	values            Values
	revision          uint64
	submittedRevision uint64
	busy              bool
	status            string
	errors            map[string]string
	generation        uint64
	pending           Ticket
}

// New returns a ready form state.
func New() *State { return &State{status: "Ready"} }

// Edit stores the latest values and invalidates obsolete validation feedback.
func (s *State) Edit(values Values) {
	s.values = values
	s.revision++
	s.errors = nil
	if !s.busy {
		s.status = "Ready"
	}
}

// Submit validates and starts a submission if no submission is active.
func (s *State) Submit() (Ticket, bool) {
	if s.busy {
		return Ticket{}, false
	}
	s.errors = validate(s.values)
	if len(s.errors) != 0 {
		s.status = "Check the form and try again"
		return Ticket{}, false
	}
	s.generation++
	ticket := Ticket{Revision: s.revision, generation: s.generation, values: s.values}
	s.pending = ticket
	s.submittedRevision = s.revision
	s.busy = true
	s.status = "Submitting"
	return ticket, true
}

// Complete applies completion only to the currently pending submission.
func (s *State) Complete(ticket Ticket) {
	if !s.busy || ticket.generation != s.pending.generation || ticket.Revision != s.pending.Revision {
		return
	}
	s.busy = false
	s.pending = Ticket{}
	if s.revision != ticket.Revision {
		s.status = "Submission completed; newer edits were not submitted"
	} else {
		s.status = "Submitted"
	}
}

// Cancel invalidates the active submission. Repeated cancellation is harmless.
func (s *State) Cancel() {
	if !s.busy {
		return
	}
	s.busy = false
	s.generation++
	s.pending = Ticket{}
	s.status = "Canceled"
}

// Snapshot returns a detached public view of the state.
func (s *State) Snapshot() Snapshot {
	errorsCopy := make(map[string]string, len(s.errors))
	for key, value := range s.errors {
		errorsCopy[key] = value
	}
	return Snapshot{
		Revision: s.revision, SubmittedRevision: s.submittedRevision, Busy: s.busy,
		Status: s.status, Errors: errorsCopy,
	}
}

func validate(values Values) map[string]string {
	errors := make(map[string]string)
	if strings.TrimSpace(values.Name) == "" {
		errors["name"] = "Enter a name."
	}
	if strings.TrimSpace(values.Notes) == "" {
		errors["notes"] = "Enter notes."
	}
	if utf8.RuneCountInString(values.Password) < 8 {
		errors["password"] = "Use at least 8 characters for the password."
	}
	if values.Choice != "Personal" && values.Choice != "Work" {
		errors["choice"] = "Choose Personal or Work."
	}
	if !values.Accepted {
		errors["accepted"] = "Accept the terms."
	}
	return errors
}
