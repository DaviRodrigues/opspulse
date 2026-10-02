package api

import (
	"log/slog"

	"github.com/DaviRodrigues/opspulse/internal/domain"
)

type EventBroker struct {
	clients    map[chan domain.Event]bool
	register   chan chan domain.Event
	unregister chan chan domain.Event
	publish    chan domain.Event
	lastEvent  *domain.Event
}

func NewCheckBroker() *EventBroker {
	b := &EventBroker{
		clients:    make(map[chan domain.Event]bool),
		register:   make(chan chan domain.Event),
		unregister: make(chan chan domain.Event),
		publish:    make(chan domain.Event, 100),
	}
	go b.run()
	return b
}

func (b *EventBroker) Publish(e domain.Event) {
	b.publish <- e
}

func (b *EventBroker) Register(client chan domain.Event) {
	b.register <- client
}

func (b *EventBroker) UnRegister(client chan domain.Event) {
	b.unregister <- client
}

func (b *EventBroker) run() {
	for {
		select {
		case client := <-b.register:
			b.clients[client] = true
			slog.Debug("Client connected. Total clients: ", "total", len(b.clients))

			if b.lastEvent != nil {
				client <- *b.lastEvent
			}
		case client := <-b.unregister:
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client)
				slog.Info("Client disconnected. Total clients: ", "total", len(b.clients))
			}
		case data := <-b.publish:
			b.lastEvent = &data
			for client := range b.clients {
				select {
				case client <- data:
				default:
				}
			}
		}
	}
}
