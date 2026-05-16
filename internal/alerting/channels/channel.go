package channels

import (
	"context"
	"time"
)

// Alert is the payload sent to every channel.
type Alert struct {
	ID       string
	Type     string
	Severity string
	Title    string
	Summary  string
	MAC      string
	FiredAt  time.Time
	Meta     map[string]any
}

// Channel sends alerts to a specific destination.
type Channel interface {
	Name()    string
	Enabled() bool
	Send(ctx context.Context, a Alert) error
}
