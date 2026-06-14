package events

import (
	"context"
	"sync"
)

type Publisher interface {
	Publish(ctx context.Context, event Envelope) error
}

type Handler func(context.Context, Envelope) error

type Subscriber interface {
	Subscribe(topic Type, handler Handler)
}

type InMemoryBus struct {
	mu       sync.Mutex
	handlers map[Type][]Handler
	seen     map[string]struct{}
}

func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[Type][]Handler),
		seen:     make(map[string]struct{}),
	}
}

func (b *InMemoryBus) Subscribe(topic Type, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], handler)
}

func (b *InMemoryBus) Publish(ctx context.Context, event Envelope) error {
	b.mu.Lock()
	if _, ok := b.seen[event.EventID]; ok {
		b.mu.Unlock()
		return nil
	}
	b.seen[event.EventID] = struct{}{}
	handlers := append([]Handler(nil), b.handlers[event.Type]...)
	b.mu.Unlock()

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
