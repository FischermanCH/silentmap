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

type Bus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

func New() *Bus {
	return &Bus{
		handlers: make(map[EventType][]Handler),
	}
}

func (b *Bus) Subscribe(typ EventType, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[typ] = append(b.handlers[typ], h)
}

// Publish dispatches the event to all subscribers.
// Slow handlers run in their own goroutine to avoid blocking the publisher.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	handlers := make([]Handler, len(b.handlers[e.Type]))
	copy(handlers, b.handlers[e.Type])
	b.mu.RUnlock()

	for _, h := range handlers {
		h := h
		go h(e)
	}
}
