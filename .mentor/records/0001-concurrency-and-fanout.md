---
date: 2026-09-24
project: go-project
topic: concurrency-and-fanout
---

**Where they got stuck:**
- Mutação de campos dentro de loops `for _, t := range targets`, onde `t` é uma cópia por valor e não alterava a slice original.
- Gerenciamento de timeouts de requisições concorrentes sem vazar goroutines.

**What unlocked it:**
- Compreensão da semântica de cópia por valor no `range` do Go e acesso indexado `targets[idx]`.
- Adoção de `sync.WaitGroup` e posterior evolução para `wg.Go(...)` para garantir encerramento atômico das goroutines de checagem.

**Demonstrated mastery of:**
- Padrão Fan-out / Fan-in com goroutines e buffered channels.
- Implementação de `CheckURL` e `CheckAll` com `net/http` e `context.WithTimeout`.
- Testes unitários com `httptest.Server` para simular cenários de 200 OK, 500 Error e timeout.

**Left open:**
- Estratégias futuras de retry com exponential backoff e threshold anti-flapping.
