package api

import (
	"log/slog"
)

type EventBroker struct {
	clients    map[chan Event]bool
	register   chan chan Event
	unregister chan chan Event
	publish    chan Event
}

func NewCheckBroker() *EventBroker {
	b := &EventBroker{
		clients:    make(map[chan Event]bool),
		register:   make(chan chan Event),
		unregister: make(chan chan Event),
		publish:    make(chan Event, 100),
	}
	go b.run()
	return b
}

func (b *EventBroker) Publish(e Event) {
	b.publish <- e
}

func (b *EventBroker) Register(client chan Event) {
	b.register <- client
}

func (b *EventBroker) UnRegister(client chan Event) {
	b.unregister <- client
}

func (b *EventBroker) run() {
	for {
		select {
		case client := <-b.register:
			b.clients[client] = true
			slog.Info("Client connected. Total clients: ", "total", len(b.clients))
		case client := <-b.unregister:
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client)
				slog.Info("Client disconnected. Total clients: ", "total", len(b.clients))
			}
		case data := <-b.publish:
			for client := range b.clients {
				select {
				case client <- data:
				default:
				}
			}
		}
	}
}
