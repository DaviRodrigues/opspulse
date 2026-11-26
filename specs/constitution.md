# OpsPulse — Constituição de Arquitetura e Engenharia (SDD)

> Princípios invioláveis, padrões arquiteturais, restrições de dependência, segurança e fluxo de engenharia que governam todo desenvolvimento no **OpsPulse**.

---

## 1. Identidade e Propósito do Sistema

O **OpsPulse** é um sistema de monitoramento ativo de infraestrutura, API REST com streaming em tempo real (**Server-Sent Events — SSE**), Dashboard Web (**React + TypeScript + Vite**) e ChatOps (**Discord Bot**), executado em produção em servidor local para validar a saúde e disponibilidade de outros serviços.

---

## 2. Arquitetura Pragmática em Go (Clean sem Burocracia)

Adotamos uma **Arquitetura Pragmática em Go**: direta ao ponto, com separação clara de responsabilidades, alta testabilidade por interfaces e sem camadas artificiais ou DTOs burocráticos.

### 2.1 Estrutura Canônica de Pacotes

- `cmd/api/` — Binário do servidor HTTP REST/SSE e loop de monitoramento da API.
- `cmd/opspulse/` — Binário do monitor local e integração ChatOps com Discord.
- `cmd/database/` — Executor de migrações versionadas (`golang-migrate/migrate/v4`).
- `internal/domain/` — Entidades puras (`Target`, `CheckLogs`/`CheckLog`, `CheckResult`, `Event`, `Incident`) e erros de domínio (`ErrNotFound`, `ErrServiceDown`, etc.), sem acoplamento a transporte HTTP ou driver de banco.
- `internal/database/` — Conexão MongoDB (`MongoClient`), contratos de repositório (`TargetRepository`, `CheckLogRepository`), implementações concretas (`MongoDB*Repository`) e fakes/mocks em memória (`Mock*Repository`).
- `internal/checker/` — Motor de health check HTTP concorrente, agendamento com jitter estocástico e abstração `Notifier`.
- `internal/api/` — Camada de transporte HTTP (`chi/v5`), middlewares (logging, recovery, CORS), handlers REST, `EventBroker` (SSE Pub/Sub) e documentação Scalar/OpenAPI.
- `internal/discord/` — Bot Discord (`discordgo`), comandos (`/status`), botões interativos (`btn_recheck`) e formatação de embeds.
- `internal/config/` — Carregamento tipado de configuração por domínio (`AppConfig`, `ServerConfig`, `DatabaseConfig`, `MonitorConfig`, `DiscordConfig`, `LogConfig`) com fallbacks seguros.
- `internal/file/` — Leitura de `.env` (`godotenv`) e loaders legados/auxiliares (JSON/YAML).
- `internal/logger/` — Inicialização de `log/slog` estruturado com suporte a rotação/diretório por serviço.
- `migrations/` — Scripts versionados de criação de coleções e índices no MongoDB.
- `web/opspulse-frontend/` — Dashboard SPA em React 19, TypeScript, Tailwind CSS v4 e Vite.

### 2.2 Regras Estruturais

1. **Preservar o layout existente:** Não criar pacotes redundantes ou renomeações cosméticas (ex: não migrar `internal/database` + `internal/domain` para `internal/storage/mongo` + `internal/service` sem necessidade real de domínio).
2. **Injeção seletiva de dependências:** Construtores e handlers recebem apenas as structs de configuração e interfaces de repositório que realmente utilizam (ex: `ServerConfig`, `TargetRepository`, `CheckLogRepository`), nunca a struct `Config` global inteira.
3. **Sem estado global oculto:** Dependências (cliente MongoDB, repositórios, `EventBroker`, configurações) são explicitamente passadas na inicialização (`main.go`).

---

## 3. Política de Dependências Mínimas e Qualidade de Código (R4)

1. **Apenas bibliotecas essenciais:** É proibido adicionar dependências externas para resolver tarefas triviais que a biblioteca padrão (Go stdlib ou Web APIs/TypeScript nativo) resolve com clareza e segurança.
   - **Jitters e aleatoriedade:** Utilizar `math/rand/v2` da stdlib Go (zero dependências externas).
   - **Resiliência futura (Circuit Breaker / Backoff):** Utilizar `struct` minimalista (`ConsecutiveFailures`, `NextCheckAt`, `State`) sem frameworks pesados.
   - **Frontend:** Utilizar `axios` (requisito explícito R3) e os ícones já presentes (`lucide-react`), evitando bibliotecas pesadas de gráficos, tabelas ou utilitários para calcular insights e filtros de `check_logs`.
2. **Versões oficiais consistentes:**
   - Banco de dados: exclusivamente `go.mongodb.org/mongo-driver/v2` (`mongo` e `bson`). Remover quaisquer imports legados de `go.mongodb.org/mongo-driver/mongo` (v1).
   - Roteador HTTP: exclusivamente `github.com/go-chi/chi/v5` e `github.com/go-chi/chi/v5/middleware` (eliminar import v1 legado).
3. **Código limpo e sustentável:** Evitar *code smells*, duplicação, abstrações especulativas e gambiarras focadas apenas em "fazer funcionar" que degradem a arquitetura.

---

## 4. Fluxo de Engenharia em 5 Passos & TDD

Todo recurso, correção ou refatoração segue obrigatoriamente `docs/DEVELOPMENT_WORKFLOW.md`:

1. **Diagnóstico:** Definir escopo, restrições e critério de sucesso.
2. **Pesquisa & Trade-offs:** Consultar fontes em `docs/SOURCES_DEVELOPEMENT.md` e comparar abordagens antes de alterar código.
3. **Especificação antes do código:** Registrar/atualizar contratos em `specs/`.
4. **Ciclo TDD (`Red -> Green -> Refactor`):**
   - Escrever testes unitários falhando primeiro.
   - Implementar a solução mínima e limpa para passar.
   - Refatorar mantendo desacoplamento.
5. **Validação Contínua:**
   - **Testes Go sem dependência de banco ativo:** Toda suíte `go test ./...` deve rodar 100% offline usando mocks/fakes (`MockTargetRepository`, `MockCheckLogRepository`, `httptest.NewServer`).
   - **Detector de race conditions:** `go test -v -race ./...` deve passar sem alertas.
   - **Builds de produção:** `go build -o /dev/null ./cmd/api` e `go build -o /dev/null ./cmd/opspulse` (e `./cmd/database`) devem compilar sem erros.
   - **Frontend:** `npm run lint` (`oxlint`) e `npm run build` (`tsc -b && vite build`) devem passar sem erros.

---

## 5. Invariantes de Concorrência, Resiliência & Graceful Shutdown

1. **Ciclo de vida orientado a Contexto:**
   - `cmd/api`, `cmd/opspulse` e `cmd/database` capturam `os.Interrupt` e `syscall.SIGTERM` via `signal.NotifyContext`.
   - Todas as goroutines de monitoramento, HTTP server e workers respeitam `ctx.Done()`, sincronizam término com `sync.WaitGroup`, executam `http.Server.Shutdown(timeoutCtx)` e fecham `MongoClient.Close(ctx)`.
2. **Streaming SSE com Actor Model (`EventBroker`):**
   - Toda mutação do mapa `clients` ocorre exclusivamente na goroutine `run()` via canais (`register`, `unregister`, `publish`).
   - O broadcast a clientes é sempre não-bloqueante (`select { case client <- data: default: }`) para impedir que um cliente lento trave o sistema inteiro.
   - O servidor HTTP mantém `SERVER_WRITE_TIMEOUT=0` para não interromper streams SSE de longa duração, enviando heartbeat periódico (`: ping\n\n`).
   - Clientes React devem fechar `EventSource` (`es.close()`) no cleanup do `useEffect`.
3. **Compatibilidade de Contrato SSE & Discord:**
   - `domain.CheckResult` mantém suas chaves JSON em PascalCase (`Name`, `URL`, `StatusCode`, `Latency`, `IsUp`, `Error`) para não quebrar consumidores SSE (`useSSE.ts`) e Discord (`embeds.go`).
4. **Resiliência contra Thundering Herd:**
   - Disparo de checagens aos servidores monitorados aplica **jitter estocástico de $\pm 20\%$**, evitando picos simultâneos de CPU, sockets e efeito DoS nos alvos.

---

## 6. Estratégia de Persistência — Teorema PACELC

O MongoDB foi adotado pela flexibilidade documental e suporte nativo a séries temporais (MongoDB 5.0+):

- **Sob Particionamento (P) — Consistência (C) para configurações (`targets`, futuro `users`):**
  - Operações de escrita críticas devem priorizar consistência (`WriteConcern: majority`) e integridade por índices únicos (`url` única em `targets`).
- **Em Operação Normal (E) — Baixa Latência (L) para métricas (`check_logs`):**
  - Pings em alta frequência priorizam baixa latência (`WriteConcern: 1` e escrita em lote `CreateBatch`), armazenados em coleção Time-Series (`timeField: "checked_at"`, `metaField: "target_id"`, `granularity: "seconds"`) com índice TTL automático de 30 dias (`expireAfterSeconds: 2592000`).

---

## 7. Segurança (OWASP & Twelve-Factor App)

1. **Zero Hardcoded Secrets / URLs:**
   - Portas, URIs de banco, tokens Discord, origens CORS permitidas (`CORS_ALLOWED_ORIGINS`) e URL base da API no frontend (`VITE_API_BASE_URL`) devem vir exclusivamente de variáveis de ambiente (`.env` / `.env.example` / `import.meta.env`).
2. **CORS Restritivo por Ambiente:**
   - Proibido `Access-Control-Allow-Origin: *` hardcoded em produção nos middlewares HTTP e helpers SSE (`PrepareSSE`). Origens permitidas são validadas dinamicamente contra `CORS_ALLOWED_ORIGINS`.
3. **Princípio de Menor Privilégio & Fronteira Código vs Nuvem:**
   - O backend valida entradas, limita payloads e emite headers seguros; proteções de borda distribuídas (WAF, DDoS/Rate Limiting global, Subnets privadas, TLS automático) são delegadas à camada de infraestrutura/nuvem.

---

## 8. Observabilidade e Contratos de API

- **Logs Estruturados:** Utilizar exclusivamente `log/slog` com contexto estruturado (`app`, `env`, `error`), nunca expondo tokens ou credenciais em texto claro.
- **OpenAPI 3.1 Viva:** A especificação `docs/openapi.yaml` é gerada a partir do código via `make api-docs` (`apispec -o docs/openapi.yaml`) e servida em `/docs/openapi.yaml` com interface Scalar em `/docs`.
