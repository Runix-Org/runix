package usage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// WindowGeometry stores Qt logical dimensions and, when Positioned, compositor-global logical coordinates.
type WindowGeometry struct {
	X, Y, Width, Height int
	Positioned          bool
}

// LoadWindowGeometry returns nil when no geometry has been saved.
func (s *Store) LoadWindowGeometry(ctx context.Context) (*WindowGeometry, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	defer func() { <-s.gate }()
	if s.closed {
		return nil, ErrClosed
	}
	var geometry WindowGeometry
	var positioned int
	err := s.db.QueryRowContext(ctx, `SELECT x,y,width,height,positioned FROM window_geometry WHERE id=1`).Scan(
		&geometry.X, &geometry.Y, &geometry.Width, &geometry.Height, &positioned,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load window geometry: %w", err)
	}
	if positioned != 0 && positioned != 1 {
		return nil, errors.New("load window geometry: invalid positioned value")
	}
	geometry.Positioned = positioned == 1
	return &geometry, nil
}

// SaveWindowGeometry replaces the singleton saved geometry.
func (s *Store) SaveWindowGeometry(ctx context.Context, geometry WindowGeometry) error {
	if geometry.Width <= 0 || geometry.Height <= 0 {
		return errors.New("window geometry dimensions must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.gate }()
	if s.closed {
		return ErrClosed
	}
	positioned := 0
	if geometry.Positioned {
		positioned = 1
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO window_geometry(id,x,y,width,height,positioned) VALUES(1,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET x=excluded.x,y=excluded.y,width=excluded.width,
		height=excluded.height,positioned=excluded.positioned`,
		geometry.X, geometry.Y, geometry.Width, geometry.Height, positioned)
	if err != nil {
		return fmt.Errorf("save window geometry: %w", err)
	}
	return nil
}

// ResetWindowGeometry removes only the saved geometry, preserving usage counts.
func (s *Store) ResetWindowGeometry(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return err
	}
	defer func() { <-s.gate }()
	if s.closed {
		return ErrClosed
	}
	if _, err := s.db.ExecContext(ctx, "DELETE FROM window_geometry WHERE id=1"); err != nil {
		return fmt.Errorf("reset window geometry: %w", err)
	}
	return nil
}
