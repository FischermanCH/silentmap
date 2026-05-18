// Package bus implements a simple in-process publish/subscribe event bus.
// Collectors publish events (EventDeviceSeen, etc.) and the registry
// subscribes to update device state. All operations are goroutine-safe.
package bus

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type EventType = string

const (
	EventDeviceSeen    EventType = "device.seen"
	EventDeviceNew     EventType = "device.new"
	EventDeviceLost    EventType = "device.lost"
	EventDeviceBack    EventType = "device.back"
	EventDeviceUpdated EventType = "device.updated"
	EventAlertFire     EventType = "alert.fire"
	EventAIInsight     EventType = "ai.insight"
)

type Event struct {
	ID     string
	Type   EventType
	Time   time.Time
	MAC    string
	IP     string
	Source string
	Meta   map[string]any
}

func NewEvent(typ EventType, mac, ip, source string, meta map[string]any) Event {
	return Event{
		ID:     uuid.NewString(),
		Type:   typ,
		Time:   time.Now(),
		MAC:    mac,
		IP:     ip,
		Source: source,
		Meta:   meta,
	}
}

type Handler func(Event)

// handlerEntry pairs a handler with a flag indicating whether it should run
// synchronously (blocking the publisher) or in its own goroutine.
type handlerEntry struct {
	fn   Handler
	sync bool
}

type Bus struct {
	mu       sync.RWMutex
	handlers map[EventType][]handlerEntry
}

func New() *Bus {
	return &Bus{
		handlers: make(map[EventType][]handlerEntry),
	}
}

// Subscribe registers a handler that runs asynchronously (in a goroutine).
// Use for slow consumers like alerting or AI that must not block the collector.
func (b *Bus) Subscribe(typ EventType, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[typ] = append(b.handlers[typ], handlerEntry{fn: h, sync: false})
}

// SubscribeSync registers a handler that runs synchronously on the caller's goroutine.
// Use for the Registry so that rapid ARP bursts are processed one at a time
// without spawning thousands of concurrent goroutines that all fight for SQLite.
func (b *Bus) SubscribeSync(typ EventType, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[typ] = append(b.handlers[typ], handlerEntry{fn: h, sync: true})
}

// Publish dispatches the event. Sync handlers run inline; async handlers in goroutines.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	entries := make([]handlerEntry, len(b.handlers[e.Type]))
	copy(entries, b.handlers[e.Type])
	b.mu.RUnlock()

	for _, entry := range entries {
		if entry.sync {
			entry.fn(e)
		} else {
			fn := entry.fn
			go fn(e)
		}
	}
}
