package hotkeysettings

import (
	"context"
	"fmt"
	"time"

	"runix/internal/hyprlandhotkey"
	"runix/internal/integrationsetup"
)

// UICommand is a serialized settings action. Candidates always use physical XKB codes.
type UICommand struct {
	Action      string                     `json:"action"`
	Combination hyprlandhotkey.Combination `json:"combination"`
}

// UISnapshot separates saved intent, candidate inspection, and operation errors.
type UISnapshot struct {
	Saved  *Assignment `json:"saved"`
	Status string      `json:"status"`
	Error  string      `json:"error"`
	// ErrorEvent distinguishes failures from persistent connection guidance.
	ErrorEvent      bool                        `json:"-"`
	Candidate       *hyprlandhotkey.Combination `json:"candidate"`
	CandidateStatus string                      `json:"candidate_status"`
	CandidateError  string                      `json:"candidate_error"`
	CanSave         bool                        `json:"can_save"`
	Onboarding      bool                        `json:"onboarding"`
	Completed       bool                        `json:"completed"`
	Available       bool                        `json:"available"`
}

// Controller runs on one serial worker, never on the GUI thread.
type Controller struct {
	enabled    bool
	service    *Service
	initError  error
	snapshot   UISnapshot
	current    Snapshot
	preference func() (bool, error)
	dismiss    func() error
}

func NewController(enabled bool) *Controller {
	service, err := New()
	return &Controller{
		enabled: enabled, service: service, initError: err,
		preference: integrationsetup.ShortcutSetupDismissed,
		dismiss:    integrationsetup.DismissShortcutSetup,
	}
}

// Run refreshes capabilities before accepting candidates or changing saved intent.
// integrationAvailable is the fresh product module-setup result, not installed bytes alone.
func (c *Controller) Run(ctx context.Context, command UICommand, integrationAvailable bool) UISnapshot {
	c.snapshot.Completed = false
	c.snapshot.ErrorEvent = false
	c.snapshot.Available = c.enabled && integrationAvailable
	if c.initError != nil {
		c.snapshot.Status = "degraded"
		c.snapshot.Error = c.initError.Error()
		c.snapshot.ErrorEvent = true
		return c.snapshot
	}
	switch command.Action {
	case "startup":
		c.refresh(ctx)
		dismissed, err := c.preference()
		if err != nil {
			c.snapshot.Error = "Could not read the shortcut setup preference: " + err.Error()
			c.snapshot.ErrorEvent = true
		}
		c.snapshot.Onboarding = c.snapshot.Saved == nil && !dismissed
	case "refresh":
		c.refresh(ctx)
		if c.snapshot.Candidate != nil {
			c.inspectCandidate(ctx, *c.snapshot.Candidate)
		}
	case "candidate":
		c.inspectCandidate(ctx, command.Combination)
	case "apply":
		c.apply(ctx)
	case "remove":
		c.remove(ctx)
	case "skip":
		// A preference write failure must not trap the user in setup.
		c.snapshot.Error = ""
		if err := c.dismiss(); err != nil {
			c.snapshot.Error = "Setup skipped, but the choice was not saved: " + err.Error()
			c.snapshot.ErrorEvent = true
		}
		c.snapshot.Completed = true
		c.snapshot.Onboarding = false
	}
	return c.snapshot
}

func (c *Controller) refresh(ctx context.Context) {
	c.snapshot.CanSave = false
	if !c.snapshot.Available {
		saved, _, err := c.service.store.read()
		c.snapshot.Saved = saved
		c.snapshot.Status = "unassigned"
		if saved != nil {
			c.snapshot.Status = "inactive"
		}
		c.snapshot.Error = "Integration is unavailable. Open Integration to install or reconnect the module."
		if err != nil {
			c.snapshot.Status = "degraded"
			c.snapshot.Error = err.Error()
			c.snapshot.ErrorEvent = true
		}
		return
	}
	snapshot, err := c.service.Read(ctx)
	c.observe(snapshot, err)
}

func (c *Controller) observe(snapshot Snapshot, err error) {
	c.current = snapshot
	c.snapshot.Saved = snapshot.Saved
	c.snapshot.Status = snapshot.Status
	c.snapshot.Error = snapshot.Error
	if err != nil && c.snapshot.Error == "" {
		c.snapshot.Error = err.Error()
	}
	c.snapshot.ErrorEvent = c.snapshot.Error != ""
}

func (c *Controller) inspectCandidate(ctx context.Context, combination hyprlandhotkey.Combination) {
	c.snapshot.Candidate = &combination
	c.snapshot.CandidateStatus = "unknown"
	c.snapshot.CandidateError = ""
	c.snapshot.CanSave = false
	if !c.snapshot.Available {
		c.snapshot.CandidateError = "Integration is unavailable. Open Integration and check again."
		return
	}
	if err := combination.Validate(); err != nil {
		c.snapshot.CandidateError = err.Error()
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	state, err := c.service.client.Connect(bounded)
	if err == nil {
		err = available(state)
	}
	if err != nil {
		c.snapshot.CandidateError = err.Error()
		return
	}
	inspection, err := c.service.client.Inspect(bounded, combination)
	if err != nil {
		c.snapshot.CandidateError = err.Error()
		return
	}
	switch {
	case len(inspection.Foreign) > 0 && len(inspection.Owned) > 0:
		c.snapshot.CandidateStatus = "conflict"
		c.snapshot.CandidateError = "This key may overlap another shortcut, including the runix shortcut."
	case len(inspection.Foreign) > 0:
		c.snapshot.CandidateStatus = "foreign"
		c.snapshot.CandidateError = "This key may overlap another shortcut. Choose a different shortcut."
	case inspection.State == "unknown":
		c.snapshot.CandidateError = "Could not check this shortcut. Check Integration and try again."
	case len(inspection.Owned) > 0:
		if len(inspection.Owned) == 1 && matches(inspection.Owned[0], &Assignment{
			Action: "toggle", Combination: combination,
		}) {
			c.snapshot.CandidateStatus = "owned"
			c.snapshot.CanSave = true
		} else {
			c.snapshot.CandidateStatus = "conflict"
			c.snapshot.CandidateError = "This shortcut is already used by runix."
		}
	case inspection.State == "free" && !inspection.Potential:
		c.snapshot.CandidateStatus = "free"
		c.snapshot.CanSave = true
	default:
		c.snapshot.CandidateError = "Could not check this shortcut. Choose another shortcut or try again."
	}
}

func (c *Controller) apply(ctx context.Context) {
	if c.snapshot.Candidate == nil {
		c.snapshot.Error = "Record a shortcut before saving."
		return
	}
	c.refresh(ctx)
	if !c.snapshot.Available || c.current.Token.Saved == "" || c.current.Token.Epoch == "" {
		return
	}
	c.inspectCandidate(ctx, *c.snapshot.Candidate)
	if !c.snapshot.CanSave {
		return
	}
	snapshot, err := c.service.Apply(ctx, c.current.Token, *c.snapshot.Candidate)
	c.observe(snapshot, err)
	if err != nil {
		c.snapshot.CanSave = false
		return
	}
	c.snapshot.CandidateStatus = "owned"
	c.snapshot.Completed = true
	c.snapshot.Onboarding = false
}

func (c *Controller) remove(ctx context.Context) {
	c.refresh(ctx)
	if !c.snapshot.Available || c.snapshot.Saved == nil || c.current.Token.Saved == "" {
		return
	}
	// Record deliberate removal before touching the working assignment. A failure
	// here leaves it intact, and no later startup can force setup after removal.
	if err := c.dismiss(); err != nil {
		c.snapshot.Error = fmt.Sprintf("Could not remember shortcut removal. The assignment was not changed: %v", err)
		c.snapshot.ErrorEvent = true
		return
	}
	snapshot, err := c.service.Remove(ctx, c.current.Token)
	c.observe(snapshot, err)
	c.snapshot.CanSave = false
	if err == nil {
		c.snapshot.Candidate = nil
		c.snapshot.CandidateStatus = ""
		c.snapshot.CandidateError = ""
		c.snapshot.Completed = true
		c.snapshot.Onboarding = false
	}
}
