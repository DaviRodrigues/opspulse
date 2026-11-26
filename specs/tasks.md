# OpsPulse — Plano de Tarefas por Milestone (SDD)

Lista ordenada e verificável das tarefas de entrega do **OpsPulse**, alinhada a `specs/constitution.md`, `specs/spec.md` e `specs/plan.md`.

---

## Milestone M1 — Reorganização Documental SDD (`specs/` & `docs/`)

- [x] **M1.1** Consolidar 100% do conteúdo técnico de `docs/steps/` (`api_module/PLAN.md`, `api_module/PLAN-v2.md`, `app/PLAN.md`, `database-plan/package-3-persistence.md`, `terraform-deploy-plan/README.md`) e contratos verificados no código em:
  - `specs/constitution.md`
  - `specs/spec.md`
  - `specs/plan.md`
  - `specs/tasks.md`
- [x] **M1.2** Atualizar referências quebradas em `docs/DEVELOPMENT_WORKFLOW.md` (`web_sites_sources.md` $\to$ `SOURCES_DEVELOPEMENT.md`; `docs/steps/` $\to$ `specs/`).
- [x] **M1.3** Corrigir links/typos e duplicação de seções em `README.md`, referenciando `specs/` e `docs/`.
- [x] **M1.4** Remover `docs/steps/` mantendo em `docs/` exclusivamente `DEVELOPMENT_WORKFLOW.md`, `SOURCES_DEVELOPEMENT.md` e `openapi.yaml`.
  - **Verificação:** `ls -la docs/ specs/` e `grep -rn "docs/steps\|web_sites_sources" docs/ specs/ README.md`.

---

## Milestone M2 — Domínio, Persistência MongoDB, Mocks & Checker com Jitter

> **Dependência:** M1 concluído. Seguir TDD (`docs/DEVELOPMENT_WORKFLOW.md`).

- [x] **M2.1 — Ajustar Modelos & Erros de Domínio** (`internal/domain/target.go`, `internal/domain/errors.go`)
  - Adicionar `Headers map[string]string` em `domain.Target`.
  - Adicionar `ErrInvalidInput` e `ErrAlreadyExists` em `internal/domain/errors.go`.
  - **Aceitação:** Structs compilam e preservam compatibilidade JSON/BSON/YAML e tags PascalCase de `CheckResult`.

- [x] **M2.2 — Definir Contratos & Filtros de Repositório (v2)** (`internal/database/repository.go`)
  - Remover import legado `go.mongodb.org/mongo-driver/mongo` (v1) e usar apenas `go.mongodb.org/mongo-driver/v2/mongo` e `bson`.
  - Definir `TargetFilter`, `CheckLogFilter`, `TargetRepository` (`Create`, `FindByID`, `FindAll`, `FindEnabled`, `Update`, `Delete`) e `CheckLogRepository` (`Create`, `CreateBatch`, `FindByID`, `FindAll`, `FindByTargetID`).

- [x] **M2.3 — Corrigir & Completar Mocks em Memória** (`internal/database/mock.go`)
  - Corrigir `MockTargetRepository.Create` (append em `m.order`, validação de URL única), `FindByID` (cópia defensiva da struct), e implementar `FindAll` com `TargetFilter`, `FindEnabled`, `Delete`.
  - Implementar `FindByID` e `FindAll` com `CheckLogFilter` em `MockCheckLogRepository`.
  - **Aceitação:** `MockTargetRepository` e `MockCheckLogRepository` satisfazem 100% as interfaces sem data races.

- [x] **M2.4 — Implementar Repositórios MongoDB Reais** (`internal/database/target_repository.go`, `internal/database/check_log_repository.go` ou `repository.go`)
  - Implementar `MongoDBTargetRepository` com `WriteConcern: majority`, mapeamento de `ErrNotFound` e `ErrAlreadyExists` (`mongo.IsDuplicateKeyError`).
  - Implementar `MongoDBCheckLogRepository` com `WriteConcern: 1`, `InsertMany` em `CreateBatch`, filtros dinâmicos (`target_id`, `is_up`, `status_code`, `from`, `to`, `search`) e ordenação `checked_at: -1`.

- [x] **M2.5 — Testes Unitários Offline de Persistência** (`internal/database/repository_test.go`)
  - Testar CRUD completo, paginação, filtros, `CreateBatch`, `ErrNotFound`, `ErrAlreadyExists` e acesso concorrente via mocks/fakes **sem exigir MongoDB ativo**.
  - **Verificação:** `go test -v -race ./internal/database/...`

- [x] **M2.6 — Jitter $\pm 20\%$ & Execução HTTP Rica no Checker** (`internal/checker/checker.go`, `internal/checker/checker_test.go`)
  - Implementar `ApplyJitter(base, 0.20)` com `math/rand/v2` (sem dependências externas).
  - Atualizar `checkURL` para honrar `Method`, `Headers`, `Body`, `Timeout`, `ExpectedStatus` e `ExpectedBodyMatch`.
  - Aplicar jitter entre ciclos (`StartMonitoring` com `time.NewTimer`) e dispersão entre targets em `CheckAll`.
  - Cobrir com testes unitários usando `httptest.NewServer` (jitter dentro de $\pm 20\%$, métodos/headers/body/match e `ctx.Done()`).
  - **Verificação:** `go test -v -race ./internal/checker/...`

---

## Milestone M3 — Migração para MongoDB (`cmd/api` & `cmd/opspulse`), Rotas REST, CORS & OpenAPI

> **Dependência:** M2 concluído.

- [x] **M3.1 — Configuração de CORS & Desacoplamento de Arquivo** (`internal/config/server.go`, `internal/config/monitor.go`, `.env.example`)
  - Adicionar `CORS_ALLOWED_ORIGINS` em `.env.example` e `AllowedOrigins []string` em `ServerConfig`.
  - Remover obrigatoriedade de `MONITOR_TARGETS_FILE` em `LoadMonitorConfig`.
  - Atualizar `internal/config/config_test.go`.
  - **Verificação:** `go test -v -race ./internal/config/...`

- [x] **M3.2 — CORS Configurável & Correção do Chi v5** (`internal/api/middleware.go`, `internal/api/responses.go`, `internal/api/server.go`)
  - Corrigir import `github.com/go-chi/chi/v5/middleware` em `server.go`.
  - Remover `Access-Control-Allow-Origin: *` hardcoded de `corsMiddleware` e `PrepareSSE`; validar `Origin` contra `AllowedOrigins`.

- [x] **M3.3 — Rotas & Handlers de `targets` e `check_logs` + Migração em `cmd/api`** (`internal/api/routes.go`, `internal/api/handlers.go`, `internal/api/server.go`, `cmd/api/main.go`)
  - Injetar `TargetRepository` e `CheckLogRepository` em `api.Server` e handlers.
  - Implementar `GET/POST /api/v1/targets`, `GET/PUT/PATCH/DELETE /api/v1/targets/{id}`, `GET /api/v1/check-logs`, `GET /api/v1/check-logs/{id}`.
  - Atualizar `StartMonitoring` e `GetStatus` para ler alvos via `TargetRepository.FindEnabled` e gravar resultados via `CheckLogRepository.CreateBatch`, mantendo broadcast SSE no `EventBroker`.
  - Escrever testes de API com `httptest` + mocks (`internal/api/handlers_test.go`).
  - **Verificação:** `go test -v -race ./internal/api/...`

- [x] **M3.4 — Migração de `cmd/opspulse` & Bot Discord para MongoDB** (`cmd/opspulse/main.go`, `internal/discord/bot.go`, `internal/discord/bot_test.go`)
  - Inicializar `MongoClient` e repositórios em `cmd/opspulse/main.go` com graceful shutdown (`mongoClient.Close`).
  - Substituir `cfg.Monitor.TargetURLs` por consultas dinâmicas ao `TargetRepository` tanto em `checker.StartMonitoring` quanto no callback de `/status` e `btn_recheck` do bot Discord, persistindo logs em `CheckLogRepository`.
  - **Verificação:** `go test -v -race ./...` e `go build -o /dev/null ./cmd/api && go build -o /dev/null ./cmd/opspulse && go build -o /dev/null ./cmd/database`.

- [x] **M3.5 — Regenerar Especificação OpenAPI** (`docs/openapi.yaml`)
  - Executar `make api-docs` (`apispec -o docs/openapi.yaml`) e validar UI em `GET /docs`.

---

## Milestone M4 — Frontend (`web/opspulse-frontend/`)

> **Dependência:** M3 concluído (contratos REST e CORS disponíveis).

- [x] **M4.1 — Instalação e Configuração do `axios` + SSE via `.env`** (`package.json`, `src/lib/api.ts`, `src/hooks/useSSE.ts`, `.env.example`)
  - Instalar `axios` (única nova dependência, conforme R3/R4).
  - Configurar `VITE_API_BASE_URL` sem hardcode de URLs; atualizar `useSSE` para usar `VITE_API_BASE_URL` e adicionar `es.close()` no cleanup do `useEffect`.

- [x] **M4.2 — Tipos & Camada de Serviço API** (`src/types/check.ts` / `src/types/target.ts`, `src/lib/api.ts`)
  - Tipar `Target`, `TargetPayload`, `CheckLog`, `PaginatedResponse<T>`, `CheckLogFilters` e funções de consumo REST (`targets` CRUD + `check-logs` query).

- [x] **M4.3 — Modal de Criação/Atualização de Target** (`src/components/TargetModal.tsx`)
  - Formulário completo conectado a `POST /api/v1/targets` e `PUT /api/v1/targets/{id}` (todos os campos do domínio, validação e tratamento de erros da API).

- [x] **M4.4 — Layout Principal em Cards** (`src/components/TargetCards.tsx`, `src/pages/Dashboard.tsx`)
  - Substituir `TargetTable.tsx` por grid responsivo de Cards exibindo detalhes completos do Target + status ao vivo SSE + ações (Criar, Editar, Ativar/Pausar, Excluir, Ver Logs).

- [x] **M4.5 — Página de Checklogs com Filtros & Insights** (`src/pages/CheckLogsPage.tsx`, `src/App.tsx`, `src/components/Header.tsx`)
  - Navegação entre Dashboard e Checklogs.
  - Filtros (`target_id`, `is_up`, `status_code`, período `from`/`to`, busca textual) + paginação.
  - Painel de Insights calculado em TypeScript puro (Uptime %, Latência média/min/max/P95, contagem de falhas, distribuição por status HTTP, top alvos lentos/instáveis).
  - **Verificação:** `cd web/opspulse-frontend && npm run lint && npm run build`.

---

## Milestone M6 — Intervalo por Target, Filtros Persistidos & Insight de Alvo Mais Rápido

> **Dependência:** M4 concluído. Seguir TDD (`docs/DEVELOPMENT_WORKFLOW.md`) e R4 (sem dependências extras).

- [x] **M6.1 — Atualizar Docs SDD & OpenAPI** (`specs/spec.md`, `specs/plan.md`, `specs/tasks.md`, `docs/openapi.yaml`)
  - Documentar `Target.Interval`, agendamento independente por alvo, filtros de disponibilidade com `localStorage` na Dashboard e alvo mais rápido na média nos CheckLogs.

- [x] **M6.2 — `Target.Interval` no Domínio, Repositórios & API** (`internal/domain/target.go`, `internal/database/`, `internal/api/handlers.go`, `internal/api/server.go`)
  - Adicionar `Interval time.Duration` em `domain.Target`, `TargetRequest`, `PatchTargetRequest`, `MongoDBTargetRepository.Update` e `MockTargetRepository`.
  - Validar `interval >= 0` nos handlers (rejeitando negativo com `400`), preservando `0` como fallback para `MONITOR_INTERVAL`, e sinalizar reconciliação imediata após mutações CRUD.

- [x] **M6.3 — Agendador Independente por Alvo com Jitter** (`internal/checker/checker.go`, `internal/checker/checker_test.go`, `internal/api/server.go`)
  - Implementar `RunTargetScheduler` com goroutines independentes por alvo (usando `target.Interval` ou fallback `MONITOR_INTERVAL`, + jitter $\pm 20\%$, e `target.Timeout` ou fallback `MONITOR_DEFAULT_TIMEOUT`).
  - Integrar em `checker.StartMonitoring` e `api.Server.StartMonitoring` com testes unitários comprovando cadências independentes e cancelamento limpo.

- [x] **M6.4 — Frontend: Intervalo por Target + Filtros de Disponibilidade com `localStorage`** (`src/types/check.ts`, `src/lib/targetForm.ts`, `src/components/TargetModal.tsx`, `src/lib/targetStatus.ts`, `src/components/TargetCards.tsx`)
  - Expor campo de intervalo (em segundos, vazio/`0` = padrão do servidor) no modal e exibir no card.
  - Adicionar filtros de disponibilidade (`Todos`, `UP`, `DOWN`, `Pausado`, `Aguardando`) na Dashboard persistidos no `localStorage`.

- [x] **M6.5 — Frontend: Alvo Mais Rápido na Média nos CheckLogs & Testes** (`src/lib/insights.ts`, `src/pages/CheckLogsPage.tsx`, `web/opspulse-frontend/tests/*.test.ts`)
  - Calcular `fastestTarget` (menor latência média válida) em `computeCheckLogInsights` e exibir destaque na página de CheckLogs.
  - Cobrir todas as novas funções com testes unitários e validar `go test -race ./...`, `npm test`, `npm run lint`, `npm run build` e `./tests/e2e/run_e2e.sh`.

---

## Backlog Futuro (Pós-R4)

- [ ] **F1 — Autenticação GitHub OAuth 2.0 (Cookie HTTP-Only JWT) & PATs (`SHA-256`) + Coleção `users`**
- [ ] **F2 — Hardening OWASP completo (`X-Frame-Options`, HSTS, CSP, `http.MaxBytesReader`)**
- [ ] **F3 — Backoff Exponencial ($2\times$ máx 5m), HTTP `429 Retry-After` (60s fallback) & Circuit Breaker por alvo**
- [ ] **F4 — Gestão de Incidentes (`incidents`), SLA & Anti-flapping (`AlertThreshold`)**
- [ ] **F5 — `TCPChecker`, `SSLChecker` (Watchdog TLS) & Multi-Notificadores (Slack, Telegram, Email, Teams)**
- [ ] **F6 — Observabilidade Prometheus/Grafana & Mensageria Assíncrona**
- [ ] **F7 — Provisionamento AWS EC2 (Terraform + Ansible) & CD GitHub Actions (`ghcr.io` + `production` gate)**
