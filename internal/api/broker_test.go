package api

import (
	"strings"
	"testing"
	"time"
)

func TestEventBroker_RegisterAndPublish(t *testing.T) {
	broker := NewCheckBroker()

	// 1. Cria e registra 2 canais clientes
	client1 := make(chan Event, 10)
	client2 := make(chan Event, 10)

	broker.Register(client1)
	broker.Register(client2)

	// Pequeno sleep para a goroutine do broker processar os registros
	time.Sleep(20 * time.Millisecond)

	// 2. Publica um evento de teste
	testEvent := Event{
		ID:    "101",
		Name:  "status",
		Data:  map[string]string{"service": "auth-api", "status": "UP"},
		Retry: 3000,
	}
	broker.Publish(testEvent)

	// 3. Valida que o cliente 1 recebeu o evento
	select {
	case received := <-client1:
		if received.Name != "status" || received.ID != "101" {
			t.Errorf("cliente 1 recebeu evento incorreto: %+v", received)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timeout: cliente 1 não recebeu o evento a tempo")
	}

	// 4. Valida que o cliente 2 também recebeu o mesmo evento
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

	client := make(chan Event, 10)
	broker.Register(client)
	time.Sleep(20 * time.Millisecond)

	// Desregistra o cliente
	broker.UnRegister(client)
	time.Sleep(20 * time.Millisecond)

	// Publica um novo evento
	broker.Publish(Event{Name: "alert", Data: "teste"})

	// O canal deve ter sido fechado pelo broker no UnRegister
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
	event := Event{
		ID:    "99",
		Name:  "status_update",
		Retry: 5000,
		Data: map[string]string{
			"url":   "https://github.com",
			"is_up": "true",
		},
	}

	bytes, err := formatEvent(event)
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
