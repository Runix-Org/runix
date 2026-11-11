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
		ctx      context.Context
		cancel   context.CancelFunc
	}
	LaunchController struct {
		handoff      func(context.Context, Target) (ExecutionResult, error)
		post         func(func())
		success      func(Identity)
		navigate     func(string) bool
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
	handoff func(context.Context, Target) (ExecutionResult, error), post func(func()), changed func(LaunchOutcome),
) *LaunchController {
	return &LaunchController{handoff: handoff, post: post, changed: changed, state: LaunchState{Phase: "idle"}}
}

// SetSuccessHandler installs the thread-safe accounting callback before submitting work.
// External handoffs count on the worker; internal navigation counts after GUI acknowledgement.
func (c *LaunchController) SetSuccessHandler(success func(Identity)) {
	c.success = success
}

// SetNavigationHandler installs synchronous GUI-thread navigation before submitting work.
// It returns true only when the requested form actually opens in the current window.
func (c *LaunchController) SetNavigationHandler(navigate func(string) bool) {
	c.navigate = navigate
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
		c.fail(id, "Results changed; select the item again")
		return false
	}
	_, target, ok := c.catalog.Lookup(id)
	if !ok || target.Generation != generation {
		c.fail(id, "Results changed; select the item again")
		return false
	}
	name := c.catalog.byID[id].Title
	if name == "" {
		name = id
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	a := &launchAttempt{id: id, name: name, target: target, revision: c.revision, ctx: ctx, cancel: cancel}
	c.attempt = a
	c.state = LaunchState{Phase: "submitting", ID: id, Name: name, Busy: true}
	c.emit(false)
	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		result, err := c.handoff(ctx, target)
		if err != nil || result.Navigation == "" {
			cancel()
		}
		if err == nil && result.Navigation == "" && c.success != nil {
			c.success(a.target.Identity)
		}
		if c.post != nil {
			c.post(func() { c.complete(a, result, err) })
		} else {
			cancel()
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

func (c *LaunchController) complete(a *launchAttempt, result ExecutionResult, err error) {
	if c.stopped || c.attempt != a {
		return
	}
	defer a.cancel()
	if err != nil {
		c.attempt = nil
		c.state = LaunchState{Phase: "failed", ID: a.id, Name: a.name, Error: err.Error()}
		c.emit(false)
		return
	}
	if result.Navigation != "" {
		_, current, ok := c.catalog.Lookup(a.id)
		if !c.visible || c.revision != a.revision || c.selected != a.id || a.ctx.Err() != nil ||
			!ok || current != a.target {
			c.attempt = nil
			c.state = LaunchState{Phase: "idle"}
			c.emit(false)
			return
		}
		opened := c.navigate != nil && c.navigate(result.Navigation)
		c.attempt = nil
		if !opened {
			c.fail(a.id, "Could not open the requested form")
			return
		}
		if c.success != nil {
			c.success(a.target.Identity)
		}
		c.state = LaunchState{Phase: "succeeded", ID: a.id, Name: a.name}
		c.emit(false)
		return
	}
	c.attempt = nil
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
