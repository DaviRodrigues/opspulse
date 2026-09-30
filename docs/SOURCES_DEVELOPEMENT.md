
# Fontes e Referências de Desenvolvimento — OpsPulse

Este documento reúne os materiais técnicos, guias oficiais, especificações e artigos de referência que embasam a arquitetura, o design de código e a infraestrutura do **OpsPulse**.

O objetivo é manter o projeto transparente e acessível para qualquer desenvolvedor que queira entender as decisões tomadas ou contribuir com o ecossistema.

---

## 1. Fundamentos, Idiomatismo & Estrutura em Go

Guias essenciais sobre padrões de projeto, convenções e organização de repositórios em Go.

| Recurso                              | Descrição / Para que serve                                                                              | Link                                                                                                   |
| :----------------------------------- | :-------------------------------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------- |
| **Go by Example**              | Guia prático com exemplos diretos de sintaxe, tipos, concorrência e bibliotecas padrão.                | [gobyexample.com](https://gobyexample.com/)                                                             |
| **Effective Go**               | Documentação oficial da equipe do Go ensinando a escrever código idiomático e limpo.                  | [go.dev/doc/effective_go](https://go.dev/doc/effective_go)                                              |
| **Uber Go Style Guide**        | Padrões e convenções de estilo adotados pela Uber para manter bases de código Go legíveis e seguras. | [github.com/uber-go/guide](https://github.com/uber-go/guide/blob/master/style.md)                       |
| **Standard Go Project Layout** | Padrão arquitetural de diretórios (`cmd/`, `internal/`, `build/`, `deployments/`, `docs/`).   | [github.com/golang-standards/project-layout](https://github.com/golang-standards/project-layout)        |
| **Standard Layout (pt-BR)**    | Versão traduzida em Português da estrutura padrão de pastas em projetos Go.                            | [project-layout (pt-BR)](https://github.com/golang-standards/project-layout/blob/master/README_ptBR.md) |

---

## 2. Concorrência, Actor Model & Resiliência

Padrões de concorrência com Goroutines, canais, context propagation e encerramento gracioso (*graceful shutdown*).

| Recurso                                            | Descrição / Para que serve                                                                                              | Link                                                                |
| :------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------ | :------------------------------------------------------------------ |
| **Go Concurrency: Pipelines & Cancellation** | Artigo oficial do blog do Go explicando padrões de Fan-out/Fan-in, cancelamento com`context` e canais.                 | [go.dev/blog/pipelines](https://go.dev/blog/pipelines)               |
| **Go Context Package Guide**                 | Como propagar prazos (*timeouts*), sinais de cancelamento e valores seguros entre Goroutines.                           | [go.dev/blog/context](https://go.dev/blog/context)                   |
| **Graceful Shutdown in Go HTTP Servers**     | Como capturar sinais do SO (`SIGINT`, `SIGTERM`) e encerrar conexões ativas sem derrubar requisições em andamento. | [Go HTTP Shutdown Docs](https://pkg.go.dev/net/http#Server.Shutdown) |

---

## 3. APIs REST, Roteamento & Streaming em Tempo Real (SSE)

Documentação sobre o roteador Chi, o protocolo Server-Sent Events (SSE) e manipulação de streams HTTP.

| Recurso                                        | Descrição / Para que serve                                                                                | Link                                                                                                                          |
| :--------------------------------------------- | :---------------------------------------------------------------------------------------------------------- | :---------------------------------------------------------------------------------------------------------------------------- |
| **OneUptime: Realtime SSE in Go**        | Artigo excelente demonstrando implementação de Actor Model Pub/Sub Broker para Server-Sent Events em Go.  | [oneuptime.com/.../go-realtime-applications-sse](https://oneuptime.com/blog/post/2026-02-01-go-realtime-applications-sse/view) |
| **go-chi/chi Router**                    | Roteador HTTP leve, idiomático e 100% compatível com a biblioteca padrão`net/http`.                    | [github.com/go-chi/chi](https://github.com/go-chi/chi)                                                                         |
| **MDN Web Docs: Server-Sent Events**     | Guia completo da Mozilla sobre o funcionamento do protocolo SSE e do cliente`EventSource` no browser.     | [developer.mozilla.org/.../Server-sent_events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)            |
| **W3C Server-Sent Events Specification** | Especificação técnica oficial do padrão W3C para streaming de eventos via HTTP (`text/event-stream`). | [html.spec.whatwg.org/multipage/server-sent-events.html](https://html.spec.whatwg.org/multipage/server-sent-events.html)       |

---

## 4. Containers, Docker & Hot-Reload

Estratégias de empacotamento, cache de dependências, builds multi-estágio e produtividade no desenvolvimento.

| Recurso                                     | Descrição / Para que serve                                                                                             | Link                                                                                                                             |
| :------------------------------------------ | :----------------------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------- |
| **Air — Live Reload for Go Apps**    | Utilitário de hot-reload que recompila e reinicia a aplicação automaticamente ao salvar arquivos`.go` ou `.html`. | [github.com/air-verse/air](https://github.com/air-verse/air)                                                                      |
| **Docker Official Golang Image**      | Imagens base oficiais da linguagem Go no Docker Hub (`alpine`, `bookworm`, etc.).                                    | [hub.docker.com/_/golang](https://hub.docker.com/_/golang)                                                                        |
| **Docker Multi-Stage Builds**         | Como criar imagens de produção ultra-leves (menores que 20MB) separando a fase de compilação da imagem final.        | [docs.docker.com/build/building/multi-stage/](https://docs.docker.com/build/building/multi-stage/)                                |
| **OWASP Docker Security Cheat Sheet** | Boas práticas de segurança em containers (usuário não-root, imagens mínimas, redução de superfície de ataque).   | [cheatsheetseries.owasp.org/.../Docker_Security](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html) |

---

## 5. Metodologias Cloud-Native & Boas Práticas

| Recurso                             | Descrição / Para que serve                                                                                         | Link                                                                  |
| :---------------------------------- | :------------------------------------------------------------------------------------------------------------------- | :-------------------------------------------------------------------- |
| **The Twelve-Factor App**     | Metodologia clássica para construir aplicações modernas, escaláveis e configuráveis via variáveis de ambiente. | [12factor.net/pt_br](https://12factor.net/pt_br/)                      |
| **DiscordGo Library**         | SDK Go oficial da comunidade para integração com a API e Gateway WebSocket do Discord.                             | [github.com/bwmarrin/discordgo](https://github.com/bwmarrin/discordgo) |
| **Log/Slog Standard Library** | Documentação do pacote nativo de structured logging (`log/slog`) introduzido no Go 1.21.                         | [pkg.go.dev/log/slog](https://pkg.go.dev/log/slog)                     |
