package api

import (
	"log/slog"
	"sync"

	"github.com/DaviRodrigues/opspulse/internal/domain"
)

type EventBroker struct {
	clients    map[chan domain.Event]bool
	register   chan chan domain.Event
	unregister chan chan domain.Event
	publish    chan domain.Event
	lastEvent  *domain.Event
	stop       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
}

func NewCheckBroker() *EventBroker {
	b := &EventBroker{
		clients:    make(map[chan domain.Event]bool),
		register:   make(chan chan domain.Event),
		unregister: make(chan chan domain.Event),
		publish:    make(chan domain.Event, 100),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *EventBroker) Publish(e domain.Event) {
	select {
	case b.publish <- e:
	case <-b.stop:
	}
}

func (b *EventBroker) Register(client chan domain.Event) {
	select {
	case b.register <- client:
	case <-b.stop:
		close(client)
	}
}

func (b *EventBroker) UnRegister(client chan domain.Event) {
	select {
	case b.unregister <- client:
	case <-b.stop:
	}
}

func (b *EventBroker) Close() {
	b.closeOnce.Do(func() {
		close(b.stop)
	})
	<-b.done
}

func (b *EventBroker) run() {
	defer close(b.done)
	for {
		select {
		case <-b.stop:
			for client := range b.clients {
				delete(b.clients, client)
				close(client)
			}
			return
		case client := <-b.register:
			b.clients[client] = true
			slog.Debug("Client connected. Total clients: ", "total", len(b.clients))

			if b.lastEvent != nil {
				select {
				case client <- *b.lastEvent:
				default:
				}
			}
		case client := <-b.unregister:
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client)
				slog.Info("Client disconnected. Total clients: ", "total", len(b.clients))
			}
		case data := <-b.publish:
			if data.Name == "status" {
				eventCopy := data
				b.lastEvent = &eventCopy
			}
			for client := range b.clients {
				select {
				case client <- data:
				default:
				}
			}
		}
	}
}
