package handlers

import (
	"sync"
)

type EventBus struct {
	subscribers map[string][]EventHandler
	mutex       sync.RWMutex
}

type EventHandler func(data interface{})

type Event struct {
	Type string
	Data interface{}
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]EventHandler),
	}
}

func (bus *EventBus) Subscribe(eventType string, handler EventHandler) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()
	bus.subscribers[eventType] = append(bus.subscribers[eventType], handler)
}

func (bus *EventBus) Publish(eventType string, data interface{}) {
	bus.mutex.RLock()
	handlers := bus.subscribers[eventType]
	bus.mutex.RUnlock()

	for _, handler := range handlers {
		go handler(data)
	}
}

func (bus *EventBus) Unsubscribe(eventType string) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()
	delete(bus.subscribers, eventType)
}
