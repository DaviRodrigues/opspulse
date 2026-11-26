package api

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
)

func TestEventBroker_RegisterAndPublish(t *testing.T) {
	broker := NewCheckBroker()

	client1 := make(chan domain.Event, 10)
	client2 := make(chan domain.Event, 10)

	broker.Register(client1)
	broker.Register(client2)

	// Pequeno sleep para a goroutine do broker processar os registros
	time.Sleep(20 * time.Millisecond)

	testEvent := domain.Event{
		ID:    "101",
		Name:  "status",
		Data:  map[string]string{"service": "auth-api", "status": "UP"},
		Retry: 3000,
	}
	broker.Publish(testEvent)

	select {
	case received := <-client1:
		if received.Name != "status" || received.ID != "101" {
			t.Errorf("cliente 1 recebeu evento incorreto: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout: cliente 1 não recebeu o evento a tempo")
	}

	select {
	case received := <-client2:
		if received.Name != "status" || received.ID != "101" {
			t.Errorf("cliente 2 recebeu evento incorreto: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout: cliente 2 não recebeu o evento a tempo")
	}
}

func TestEventBroker_UnRegister(t *testing.T) {
	broker := NewCheckBroker()

	client := make(chan domain.Event, 10)
	broker.Register(client)
	time.Sleep(20 * time.Millisecond)

	broker.UnRegister(client)
	time.Sleep(20 * time.Millisecond)

	broker.Publish(domain.Event{Name: "alert", Data: "teste"})

	select {
	case _, ok := <-client:
		if ok {
			t.Errorf("esperava que o canal estivesse fechado após UnRegister")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout ao verificar fechamento do canal do cliente")
	}
}

func TestFormatEvent(t *testing.T) {
	event := domain.Event{
		ID:    "99",
		Name:  "status_update",
		Retry: 5000,
		Data: map[string]string{
			"url":   "https://github.com",
			"is_up": "true",
		},
	}

	bytes, err := FormatEvent(event)
	if err != nil {
		t.Fatalf("não esperava erro ao formatar evento, recebeu: %v", err)
	}

	formatted := string(bytes)

	if !strings.Contains(formatted, "id: 99\n") {
		t.Errorf("formatação sem 'id: 99': %s", formatted)
	}

	if !strings.Contains(formatted, "event: status_update\n") {
		t.Errorf("formatação sem 'event: status_update': %s", formatted)
	}

	if !strings.Contains(formatted, "retry: 5000\n") {
		t.Errorf("formatação sem 'retry: 5000': %s", formatted)
	}

	if !strings.Contains(formatted, `data: {"is_up":"true","url":"https://github.com"}`) {
		t.Errorf("formatação sem JSON de dados correto: %s", formatted)
	}

	if !strings.HasSuffix(formatted, "\n\n") {
		t.Errorf("mensagem SSE deve terminar com '\\n\\n': %s", formatted)
	}
}

func TestEventBroker_RegisterFullChannelDoesNotBlock(t *testing.T) {
	broker := NewCheckBroker()

	broker.Publish(NewStatusEvent([]domain.CheckResult{
		{Name: "api", URL: "https://example.com", StatusCode: 200, IsUp: true},
	}))
	time.Sleep(20 * time.Millisecond)

	// Unbuffered channel (full immediately) must not block the broker actor loop
	fullClient := make(chan domain.Event)
	done := make(chan struct{})
	go func() {
		broker.Register(fullClient)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Register blocked on full client channel with lastEvent")
	}

	// Ensure broker remains responsive for subsequent clients and publishes
	healthyClient := make(chan domain.Event, 5)
	broker.Register(healthyClient)

	select {
	case ev := <-healthyClient:
		if ev.Name != "status" {
			t.Fatalf("expected cached status event, got %q", ev.Name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("broker became unresponsive after full client registration")
	}
}

func TestEventBroker_CachesOnlyStatusNotAlert(t *testing.T) {
	broker := NewCheckBroker()

	liveClient := make(chan domain.Event, 10)
	broker.Register(liveClient)
	time.Sleep(20 * time.Millisecond)

	statusEv := NewStatusEvent([]domain.CheckResult{
		{Name: "svc-a", URL: "https://a.example.com", StatusCode: 200, IsUp: true},
	})
	alertEv := NewAlertEvent(domain.CheckResult{
		Name: "svc-b", URL: "https://b.example.com", StatusCode: 503, IsUp: false, Error: "down",
	})

	broker.Publish(statusEv)
	broker.Publish(alertEv)

	// Live client receives both status and alert immediately
	var received []string
	for range 2 {
		select {
		case ev := <-liveClient:
			received = append(received, ev.Name)
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("timed out waiting for live events; got %v", received)
		}
	}
	if received[0] != "status" || received[1] != "alert" {
		t.Fatalf("expected [status, alert], got %v", received)
	}

	// Late-joining client receives the cached "status" snapshot, NOT the "alert"
	lateClient := make(chan domain.Event, 10)
	broker.Register(lateClient)

	select {
	case ev := <-lateClient:
		if ev.Name != "status" {
			t.Fatalf("expected late client to receive cached 'status' event, got %q", ev.Name)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for cached status event on late client")
	}
}

func TestEventBroker_ConcurrentRegisterUnregisterDoubleUnregisterPublish(t *testing.T) {
	broker := NewCheckBroker()
	defer broker.Close()

	const workers = 20
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(idx int) {
			defer wg.Done()
			ch := make(chan domain.Event, 4)
			broker.Register(ch)
			broker.Publish(NewStatusEvent([]domain.CheckResult{
				{Name: "svc", URL: "https://example.com", StatusCode: 200, IsUp: true},
			}))
			if idx%2 == 0 {
				broker.Publish(NewAlertEvent(domain.CheckResult{
					Name: "svc", URL: "https://example.com", StatusCode: 503, IsUp: false, Error: "err",
				}))
			}
			broker.UnRegister(ch)
			broker.UnRegister(ch) // double unregister must not panic
		}(i)
	}
	wg.Wait()
}
