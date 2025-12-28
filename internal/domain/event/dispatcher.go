package event

import (
	"sync"
)

// EventHandler handles domain events
type EventHandler interface {
	// Handle processes the event
	Handle(event DomainEvent) error
	// HandledEvents returns the event names this handler handles
	HandledEvents() []string
}

// EventDispatcher dispatches domain events to registered handlers
type EventDispatcher interface {
	// Dispatch sends an event to all registered handlers
	Dispatch(event DomainEvent)
	// DispatchAll dispatches multiple events
	DispatchAll(events []DomainEvent)
	// Subscribe registers a handler for events
	Subscribe(handler EventHandler)
	// Unsubscribe removes a handler
	Unsubscribe(handler EventHandler)
}

// InMemoryDispatcher is an in-memory implementation of EventDispatcher
type InMemoryDispatcher struct {
	handlers map[string][]EventHandler
	mu       sync.RWMutex
	async    bool
}

// NewInMemoryDispatcher creates a new InMemoryDispatcher
func NewInMemoryDispatcher(async bool) *InMemoryDispatcher {
	return &InMemoryDispatcher{
		handlers: make(map[string][]EventHandler),
		async:    async,
	}
}

// Dispatch sends an event to all registered handlers
func (d *InMemoryDispatcher) Dispatch(event DomainEvent) {
	d.mu.RLock()
	handlers := d.handlers[event.EventName()]
	// Also get handlers registered for all events
	allHandlers := d.handlers["*"]
	d.mu.RUnlock()

	combinedHandlers := append(handlers, allHandlers...)

	for _, handler := range combinedHandlers {
		if d.async {
			go func(h EventHandler) {
				_ = h.Handle(event)
			}(handler)
		} else {
			_ = handler.Handle(event)
		}
	}
}

// DispatchAll dispatches multiple events
func (d *InMemoryDispatcher) DispatchAll(events []DomainEvent) {
	for _, event := range events {
		d.Dispatch(event)
	}
}

// Subscribe registers a handler for events
func (d *InMemoryDispatcher) Subscribe(handler EventHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, eventName := range handler.HandledEvents() {
		d.handlers[eventName] = append(d.handlers[eventName], handler)
	}
}

// Unsubscribe removes a handler
func (d *InMemoryDispatcher) Unsubscribe(handler EventHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for _, eventName := range handler.HandledEvents() {
		handlers := d.handlers[eventName]
		for i, h := range handlers {
			if h == handler {
				d.handlers[eventName] = append(handlers[:i], handlers[i+1:]...)
				break
			}
		}
	}
}

// NullDispatcher is a no-op dispatcher for when events are not needed
type NullDispatcher struct{}

// NewNullDispatcher creates a new NullDispatcher
func NewNullDispatcher() *NullDispatcher {
	return &NullDispatcher{}
}

// Dispatch does nothing
func (d *NullDispatcher) Dispatch(event DomainEvent) {}

// DispatchAll does nothing
func (d *NullDispatcher) DispatchAll(events []DomainEvent) {}

// Subscribe does nothing
func (d *NullDispatcher) Subscribe(handler EventHandler) {}

// Unsubscribe does nothing
func (d *NullDispatcher) Unsubscribe(handler EventHandler) {}
