package desktop

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"runix/internal/launchable"
	"runix/internal/search"
)

// Provider adapts the desktop catalog to launchable's provider contract.
type Provider struct {
	mu        sync.Mutex
	config    CatalogConfig
	launcher  *Launcher
	live      *Live
	current   *Snapshot
	projected *launchable.Snapshot
	publish   func(launchable.ProviderUpdate)
	started   bool
	closed    bool
	closeOnce sync.Once
}

// NewProvider captures scan and launch inputs. It performs no discovery until Start.
func NewProvider(config CatalogConfig, environment []string) *Provider {
	config.Roots = append([]string(nil), config.Roots...)
	config.Diagnostics = append([]Diagnostic(nil), config.Diagnostics...)
	return &Provider{config: config, launcher: NewLauncher(environment)}
}

func (p *Provider) ID() string { return "desktop" }

// Start starts asynchronous discovery and live observation.
func (p *Provider) Start(ctx context.Context, publish func(launchable.ProviderUpdate)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return fmt.Errorf("desktop provider is closed")
	}
	if p.started {
		return fmt.Errorf("desktop provider is already started")
	}
	p.started = true
	p.publish = publish
	p.live = StartLive(ctx, p.config, func(update Update) {
		_ = p.install(update.Snapshot, update.InvalidatedIcons)
	})
	return nil
}

func (p *Provider) install(snapshot *Snapshot, invalidated []string) error {
	if snapshot == nil {
		return fmt.Errorf("desktop provider received an empty snapshot")
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("desktop provider is closed")
	}
	if p.current == snapshot {
		publish, projected := p.publish, p.projected
		p.mu.Unlock()
		if publish != nil && len(invalidated) != 0 {
			publish(launchable.ProviderUpdate{Snapshot: projected, InvalidatedIcons: invalidated})
		}
		return nil
	}
	p.mu.Unlock()
	items := snapshot.Applications()
	projected := make([]launchable.Item, 0, len(items))
	for _, app := range items {
		entry := app.Entry
		name := fallback(entry.Name.Selected, entry.Name.Base)
		generic := fallback(entry.GenericName.Selected, entry.GenericName.Base)
		subtitle := generic
		if subtitle == "" {
			subtitle = fallback(entry.Comment.Selected, entry.Comment.Base)
		}
		icon := fallback(entry.Icon.Selected, entry.Icon.Base)
		item := launchable.Item{ID: app.Source.ID, Title: name, Subtitle: subtitle}
		if filepath.IsAbs(icon) && !strings.ContainsRune(icon, '\x00') {
			item.IconPath = icon
		} else if validThemeIcon(icon) {
			item.IconName = icon
		}
		item.Fields = []search.Field{
			{Kind: search.FieldName, Value: entry.Name.Base},
			{Kind: search.FieldName, Value: entry.Name.Selected},
			{Kind: search.FieldGenericName, Value: entry.GenericName.Base},
			{Kind: search.FieldGenericName, Value: entry.GenericName.Selected},
			{Kind: search.FieldID, Value: app.Source.ID},
		}
		for _, value := range entry.Keywords.Base {
			item.Fields = append(item.Fields, search.Field{Kind: search.FieldKeyword, Value: value})
		}
		for _, value := range entry.Keywords.Selected {
			item.Fields = append(item.Fields, search.Field{Kind: search.FieldKeyword, Value: value})
		}
		projected = append(projected, item)
	}
	diagnostics := snapshot.Diagnostics()
	converted := make([]launchable.Diagnostic, len(diagnostics))
	for i, diagnostic := range diagnostics {
		converted[i] = launchable.Diagnostic{
			ProviderID: p.ID(), Code: diagnostic.Code, Path: diagnostic.Path,
			ID: diagnostic.ID, Message: diagnostic.Message,
		}
	}
	immutable, err := launchable.NewSnapshot(snapshot.Generation(), projected, converted)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("desktop provider is closed")
	}
	p.current = snapshot
	p.projected = immutable
	publish := p.publish
	p.mu.Unlock()
	if publish != nil {
		publish(launchable.ProviderUpdate{Snapshot: immutable, InvalidatedIcons: append([]string(nil), invalidated...)})
	}
	return nil
}

// Execute validates and captures the current visible record atomically with snapshot publication.
func (p *Provider) Execute(ctx context.Context, itemID string, generation uint64) error {
	p.mu.Lock()
	if p.closed || p.current == nil || p.current.Generation() != generation {
		p.mu.Unlock()
		return fmt.Errorf("desktop application selection is stale")
	}
	app, exists := p.current.Lookup(itemID)
	if !exists || !app.Visible || !app.Valid {
		p.mu.Unlock()
		return fmt.Errorf("desktop application is no longer visible")
	}
	entry := app.Entry
	p.mu.Unlock()
	return p.launcher.Launch(ctx, entry)
}

// Close stops live observation and joins launcher work; repeated calls are safe.
func (p *Provider) Close() {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		live := p.live
		p.mu.Unlock()
		if live != nil {
			live.Close()
		}
		p.launcher.Close()
	})
}

func fallback(selected, base string) string {
	if selected != "" {
		return selected
	}
	return base
}

func validThemeIcon(value string) bool {
	if value == "" || filepath.IsAbs(value) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return false
		}
	}
	return true
}
