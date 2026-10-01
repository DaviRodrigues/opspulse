---
date: 2026-09-26
project: go-project
topic: sse-broker-and-http-lifecycle
---

**Where they got stuck:**
- Orquestração de inicialização no `server.Setup(ctx)`: a chamada bloqueante `s.Start()` antes de `StartMonitoring` impedia o início do loop de checagem.
- O `WriteTimeout: 5 * time.Second` do `http.Server` encerrava prematuramente conexões de Server-Sent Events (streaming de longa duração).
- Clientes SSE conectando ficavam sem dados até o próximo tick do monitor por falta de um snapshot inicial.

**What unlocked it:**
- Disparo do `StartMonitoring` em paralelo com goroutines gerenciadas por `sync.WaitGroup` / `wg.Go`.
- Configuração de `WriteTimeout: 0` (ou desativação de timeout de escrita) no servidor HTTP para suportar conexões SSE persistentes.
- Envio imediato do estado atual (`s.broker.Notify(snapshot)`) ao aceitar nova subscrição SSE no handler.

**Demonstrated mastery of:**
- Arquitetura Pub/Sub baseada em Actor Model usando Go channels sem contenção de mutex.
- Gerenciamento seguro de ciclo de vida de clientes (registro, transmissão de eventos e cancelamento via `ctx.Done()`).
- Formatação de streams de eventos de acordo com a especificação W3C SSE (`data: ...\n\n`).
- Testes unitários com simulação concorrente de múltiplos subscribers no `broker_test.go`.

**Left open:**
- Estratégia de backpressure quando clientes lentos (slow consumers) demoram a consumir mensagens do canal buffered.
