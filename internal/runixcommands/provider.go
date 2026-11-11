// Package runixcommands supplies the statically registered internal launcher commands.
package runixcommands

import (
	"context"
	"errors"
	"sync"

	"runix/internal/launchable"
	"runix/internal/search"
)

// Provider owns the immutable internal command catalog; GUI adapters perform navigation.
type Provider struct {
	mu      sync.Mutex
	started bool
	closed  bool
}

func NewProvider() *Provider { return &Provider{} }

func (p *Provider) ID() string { return "runix" }

func (p *Provider) Start(ctx context.Context, publish func(launchable.ProviderUpdate)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	if p.started || p.closed {
		p.mu.Unlock()
		return errors.New("internal command provider is already started or closed")
	}
	p.started = true
	p.mu.Unlock()
	snapshot, err := launchable.NewSnapshot(1, []launchable.Item{{
		ID: "settings", Title: "Settings", Subtitle: "runix settings", IconName: "runix-settings",
		Type: "Command", PrimaryAction: "Open settings",
		Fields: []search.Field{
			{Kind: search.FieldName, Value: "Settings"},
			{Kind: search.FieldKeyword, Value: "Preferences"},
			{Kind: search.FieldKeyword, Value: "Настройки"},
		},
	}}, nil)
	if err != nil {
		return err
	}
	publish(launchable.ProviderUpdate{Snapshot: snapshot})
	return nil
}

func (p *Provider) Execute(ctx context.Context, id string, generation uint64) (launchable.ExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return launchable.ExecutionResult{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started || p.closed || id != "settings" || generation != 1 {
		return launchable.ExecutionResult{}, errors.New("internal command selection is stale")
	}
	return launchable.ExecutionResult{Navigation: "settings"}, nil
}

func (p *Provider) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
}
