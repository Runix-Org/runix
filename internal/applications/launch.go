package applications

import (
	"context"
	"sync"

	"runix/internal/desktop"
)

// LaunchState is the owner-thread view of one launch interaction.
type LaunchState struct {
	Phase string
	ID    string
	Name  string
	Error string
	Busy  bool
}

// LaunchOutcome carries state changes; Hide is a completion-only hide request.
type LaunchOutcome struct {
	State LaunchState
	Hide  bool
}

type launchAttempt struct {
	id, name string
	entry    desktop.Entry
	revision uint64
	cancel   context.CancelFunc
}

// LaunchController owns the GUI-thread launch state and one bounded handoff worker.
type LaunchController struct {
	handoff      func(context.Context, desktop.Entry) error
	post         func(func())
	changed      func(LaunchOutcome)
	catalog      *Catalog
	state        LaunchState
	selected     string
	visible      bool
	revision     uint64
	resetPending bool
	attempt      *launchAttempt
	stopped      bool
	workers      sync.WaitGroup
}

// NewLaunchController uses an owner-thread dispatcher for all state notifications.
func NewLaunchController(
	handoff func(context.Context, desktop.Entry) error, post func(func()), changed func(LaunchOutcome),
) *LaunchController {
	return &LaunchController{handoff: handoff, post: post, changed: changed, state: LaunchState{Phase: "idle"}}
}

func (c *LaunchController) emit(hide bool) {
	if c.changed != nil {
		c.changed(LaunchOutcome{State: c.state, Hide: hide})
	}
}

func (c *LaunchController) SetCatalog(catalog *Catalog) {
	if c.stopped {
		return
	}
	c.catalog = catalog
}

func (c *LaunchController) Select(id string) {
	if c.stopped {
		return
	}
	if c.selected == id {
		return
	}
	c.selected = id
	if c.state.Error != "" {
		c.state = LaunchState{Phase: "idle", ID: id}
		c.emit(false)
	}
}

func (c *LaunchController) Submit(id string, generation uint64) bool {
	if c.stopped || c.attempt != nil {
		return false
	}
	c.selected = id
	if c.catalog == nil || generation != c.catalog.Generation() {
		c.fail(id, "Application list changed; select the application again")
		return false
	}
	record, ok := c.catalog.Lookup(id)
	if !ok || !record.Visible || !record.Valid {
		c.fail(id, "Application is unavailable")
		return false
	}
	entry := record.Entry
	name := entry.Name.Selected
	if name == "" {
		name = entry.Name.Base
	}
	if name == "" {
		name = id
	}
	ctx, cancel := context.WithTimeout(context.Background(), desktop.HandoffTimeout)
	a := &launchAttempt{id: id, name: name, entry: entry, revision: c.revision, cancel: cancel}
	c.attempt = a
	c.state = LaunchState{Phase: "submitting", ID: id, Name: name, Busy: true}
	c.emit(false)
	c.workers.Go(func() { c.run(ctx, a) })
	return true
}

func (c *LaunchController) fail(id, message string) {
	name := ""
	if c.catalog != nil {
		if record, ok := c.catalog.Lookup(id); ok {
			name = record.Entry.Name.Selected
			if name == "" {
				name = record.Entry.Name.Base
			}
		}
	}
	c.state = LaunchState{Phase: "failed", ID: id, Name: name, Error: message}
	c.emit(false)
}

func (c *LaunchController) run(ctx context.Context, attempt *launchAttempt) {
	err := c.handoff(ctx, attempt.entry)
	attempt.cancel()
	if c.post != nil {
		c.post(func() { c.complete(attempt, err) })
	}
}

func (c *LaunchController) complete(attempt *launchAttempt, err error) {
	if c.stopped || c.attempt != attempt {
		return
	}
	c.attempt = nil
	if err != nil {
		c.state = LaunchState{Phase: "failed", ID: attempt.id, Name: attempt.name, Error: err.Error()}
		c.emit(false)
		return
	}
	c.state = LaunchState{Phase: "succeeded", ID: attempt.id, Name: attempt.name}
	c.resetPending = true
	c.emit(c.visible && c.revision == attempt.revision)
}

func (c *LaunchController) VisibilityChanged(visible bool) {
	if c.stopped || c.visible == visible {
		return
	}
	c.visible = visible
	c.revision++
}

// BeforeShow consumes a success reset once. The adapter clears query and selection,
// then submits an empty query when true.
func (c *LaunchController) BeforeShow() bool {
	if c.stopped || !c.resetPending {
		return false
	}
	c.resetPending = false
	return true
}

func (c *LaunchController) Stop() {
	if c.stopped {
		return
	}
	c.stopped = true
	if c.attempt != nil {
		c.attempt.cancel()
		c.attempt = nil
	}
	c.workers.Wait()
}
