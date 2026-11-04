package launchable

import (
	"context"
	"sync"
	"time"
)

type (
	LaunchState struct {
		Phase, ID, Name, Error string
		Busy                   bool
	}
	LaunchOutcome struct {
		State LaunchState
		Hide  bool
	}
	launchAttempt struct {
		id, name string
		target   Target
		revision uint64
		cancel   context.CancelFunc
	}
	LaunchController struct {
		handoff      func(context.Context, Target) error
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
)

func NewLaunchController(
	handoff func(context.Context, Target) error, post func(func()), changed func(LaunchOutcome),
) *LaunchController {
	return &LaunchController{handoff: handoff, post: post, changed: changed, state: LaunchState{Phase: "idle"}}
}

func (c *LaunchController) emit(hide bool) {
	if c.changed != nil {
		c.changed(LaunchOutcome{State: c.state, Hide: hide})
	}
}

func (c *LaunchController) SetCatalog(v *Catalog) {
	if !c.stopped {
		c.catalog = v
	}
}

func (c *LaunchController) Select(id string) {
	if c.stopped || c.selected == id {
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
	if c.catalog == nil {
		c.fail(id, "Application list changed; select the application again")
		return false
	}
	_, target, ok := c.catalog.Lookup(id)
	if !ok || target.Generation != generation {
		c.fail(id, "Application list changed; select the application again")
		return false
	}
	name := c.catalog.byID[id].Title
	if name == "" {
		name = id
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	a := &launchAttempt{id: id, name: name, target: target, revision: c.revision, cancel: cancel}
	c.attempt = a
	c.state = LaunchState{Phase: "submitting", ID: id, Name: name, Busy: true}
	c.emit(false)
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		err := c.handoff(ctx, target)
		cancel()
		if c.post != nil {
			c.post(func() { c.complete(a, err) })
		}
	}()
	return true
}

func (c *LaunchController) fail(id, msg string) {
	name := ""
	if c.catalog != nil {
		if r, _, ok := c.catalog.Lookup(id); ok {
			name = r.Title
		}
	}
	c.state = LaunchState{Phase: "failed", ID: id, Name: name, Error: msg}
	c.emit(false)
}

func (c *LaunchController) complete(a *launchAttempt, err error) {
	if c.stopped || c.attempt != a {
		return
	}
	c.attempt = nil
	if err != nil {
		c.state = LaunchState{Phase: "failed", ID: a.id, Name: a.name, Error: err.Error()}
		c.emit(false)
		return
	}
	c.state = LaunchState{Phase: "succeeded", ID: a.id, Name: a.name}
	c.resetPending = true
	c.emit(c.visible && c.revision == a.revision)
}

func (c *LaunchController) VisibilityChanged(v bool) {
	if c.stopped || c.visible == v {
		return
	}
	c.visible = v
	c.revision++
}

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
