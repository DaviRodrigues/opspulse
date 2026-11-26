# OpsPulse — Especificação Funcional e Técnica (SDD)

Este documento consolida todas as especificações funcionais, contratos de interface e requisitos não funcionais do **OpsPulse**, claramente divididos entre o **Escopo Atual de Implementação (R1–R4)** e o **Roadmap Futuro (Pós-R4)**.

---

# PARTE I — ESCOPO ATUAL DE IMPLEMENTAÇÃO (R1–R4)

## 1. Visão Geral do Escopo Atual

O escopo imediato entrega:
- **R1:** Reorganização documental para o padrão SDD em `specs/` (`constitution.md`, `spec.md`, `plan.md`, `tasks.md`) e limpeza de `docs/`.
- **R2:** Persistência completa em MongoDB (`targets` e `check_logs`) com repositórios reais + testes unitários com mocks/fakes (sem MongoDB ativo), substituição de arquivos Target em `cmd/api` e `cmd/opspulse`, rotas REST completas para `targets` e `check_logs`, CORS configurável via `.env`, e jitter estocástico no `internal/checker`.
- **R3:** Atualização do frontend em `web/opspulse-frontend` com `axios` via `.env`, modal de criação/atualização de Target sem hardcode, layout principal em cards com detalhes ricos, e página dedicada de `check_logs` com filtros e insights.
- **R4:** Dependências mínimas essenciais e código limpo, flexível e sem *code smells*.

---

## 2. Modelos de Domínio (`internal/domain/`)

### 2.1 `Target` (`internal/domain/target.go`)

Representa um serviço monitorado persistido na coleção `targets`.

| Campo Go | Tipo | Tag JSON | Tag BSON | Observação / Delta Atual |
| :--- | :--- | :--- | :--- | :--- |
| `ID` | `bson.ObjectID` | `id,omitempty` | `_id,omitempty` | Chave primária MongoDB |
| `Name` | `string` | `name` | `name` | Nome amigável do serviço |
| `URL` | `string` | `url` | `url` | Obrigatória, HTTP/HTTPS válida, índice único |
| `Method` | `string` | `method` | `method` | `GET`, `POST`, `PUT`, `HEAD`, etc. (default `GET`) |
| `Headers` | `map[string]string` | `headers,omitempty` | `headers,omitempty` | **Delta obrigatório:** ausente no `target.go` atual; exigido por `target_example.json` e especificação de persistência |
| `Enabled` | `bool` | `enabled` | `enabled` | Controla se o alvo participa das checagens |
| `Tags` | `[]string` | `tags` | `tags` | Categorização livre |
| `Body` | `map[string]any` | `body` | `body` | Payload opcional para `POST`/`PUT` |
| `Timeout` | `time.Duration` | `timeout` | `timeout` | Duração por requisição (nanosegundos no JSON Go padrão; `0` usa `MONITOR_DEFAULT_TIMEOUT`, fallback `5s`) |
| `Interval` | `time.Duration` | `interval` | `interval` | Intervalo individual de checagem por alvo (nanosegundos; `0` herda `MONITOR_INTERVAL` do servidor, fallback `30s`) |
| `ExpectedStatus` | `[]int` | `expected_status` | `expected_status` | Default `[200, 201, 204]` ou `200..399` se vazio |
| `ExpectedBodyMatch` | `string` | `expected_body_match` | `expected_body_match` | Substring/padrão esperado no corpo da resposta |
| `CreatedAt` | `time.Time` | `created_at` | `created_at` | Preenchido no `Create` |
| `UpdatedAt` | `time.Time` | `updated_at` | `updated_at` | Atualizado no `Create` e `Update` |

> **Nota sobre `TargetResult`:** `domain.TargetResult` permanece compatível com `internal/file` e `internal/checker` para retrocompatibilidade, devendo ser mapeado de/para `domain.Target` (ou `checker` passar a aceitar/operar com `domain.Target` preservando `TargetID` para gravar em `check_logs`).

### 2.2 `CheckLogs` / `CheckLog` (`internal/domain/check_logs.go`)

Representa um registro histórico de checagem persistido na coleção Time-Series `check_logs`.

| Campo Go | Tipo | Tag JSON | Tag BSON | Observação |
| :--- | :--- | :--- | :--- | :--- |
| `ID` | `bson.ObjectID` | `id,omitempty` | `_id,omitempty` | Gerado no `Create`/`CreateBatch` |
| `TargetID` | `bson.ObjectID` | `target_id,omitempty` | `target_id,omitempty` | `metaField` da coleção Time-Series |
| `Name` | `string` | `name` | `name` | Nome do serviço no momento do check |
| `URL` | `string` | `url` | `url` | URL verificada |
| `StatusCode` | `int` | `status_code` | `status_code` | Código HTTP retornado (`0` em erro de rede) |
| `Latency` | `time.Duration` | `latency` | `latency` | Armazenado em `time.Duration` (ns no JSON Go). No frontend, converter `latency / 1_000_000` para exibir em ms (ver nota de contrato abaixo). |
| `IsUp` | `bool` | `is_up` | `is_up` | Resultado booleano da saúde |
| `Error` | `string` | `error,omitempty` | `error,omitempty` | Mensagem de diagnóstico quando `!IsUp` |
| `CheckedAt` | `time.Time` | `checked_at` | `checked_at` | `timeField` da coleção Time-Series (UTC) |

- Alias existente mantido: `type CheckLog = CheckLogs`.
- **Contrato de `Latency` vs `latency_ms`:** Os documentos iniciais de banco listavam `LatencyMs` conceitualmente, enquanto o domínio implementado (`internal/domain/check_logs.go`) usa `Latency time.Duration` com tag `json:"latency" bson:"latency"`. Mantém-se `Latency time.Duration` no domínio e JSON (`latency` em nanosegundos, consistente com `CheckResult.Latency`), documentando explicitamente no OpenAPI e no cliente TypeScript a conversão `(latency / 1_000_000)` para milissegundos.

### 2.3 `CheckResult` (`internal/domain/check_logs.go`)

Usado para streaming SSE e embeds do Discord. **Invariante:** manter tags JSON em **PascalCase** (`Name`, `URL`, `StatusCode`, `Latency`, `IsUp`, `Error`) para compatibilidade com `useSSE.ts` e `discord/embeds.go`.

### 2.4 `Event` (`internal/domain/event.go`) & Erros (`internal/domain/errors.go`)

- `Event`: `ID string` (`id,omitempty`), `Name string` (`name`), `Data any` (`data`), `Retry int` (`retry,omitempty`).
- Erros de domínio existentes: `ErrServiceDown`, `ErrDiscordAuth`, `ErrNotFound`, `ErrInvalidInterval`, `ErrInvalidFileFormat`.
- Novos erros recomendados para validação/persistência na API: `ErrInvalidInput` (validação de payload/ObjectID) e `ErrAlreadyExists` (violação de índice único de `url` em `targets`, mapeado para HTTP `409 Conflict`).

---

## 3. Persistência MongoDB, Migrações & Repositórios (`internal/database/`, `migrations/`)

### 3.1 Driver, Migrações & Índices

- **Driver:** `go.mongodb.org/mongo-driver/v2/mongo` e `go.mongodb.org/mongo-driver/v2/bson`.
  - **Delta atual:** `internal/database/repository.go` ainda importa `go.mongodb.org/mongo-driver/mongo` (v1) e possui apenas assinaturas incompletas sem corpo; deve ser corrigido para v2.
- **Migrações (`migrations/` + `cmd/database/main.go` via `golang-migrate/migrate/v4`):**
  - `000001_create_targets.up.json`: cria coleção `targets` com índices:
    - `idx_targets_url_unique`: `{ "url": 1 }`, `unique: true`
    - `idx_targets_created_at`: `{ "created_at": -1 }`
  - `000002_create_check_logs.up.json`: cria coleção Time-Series `check_logs`:
    ```json
    {
      "create": "check_logs",
      "timeseries": {
        "timeField": "checked_at",
        "metaField": "target_id",
        "granularity": "seconds"
      },
      "expireAfterSeconds": 2592000
    }
    ```
    (TTL de 30 dias = `2592000` segundos).
  - `000003_create_incidents.up.json`: cria coleção `incidents` (reservada para uso no Roadmap Pós-R4).
- **Garantia de índices / fallback:** Caso o serviço suba contra um banco sem `go-migrate` executado manualmente ou em ambiente local novo, `EnsureIndexes` / inicialização segura não deve quebrar coleções Time-Series já existentes.

### 3.2 Contratos de Repositório (`internal/database/`)

#### `TargetRepository`

```go
type TargetFilter struct {
	Enabled *bool
	Tag     string
	Search  string // match parcial case-insensitive em Name ou URL
}

type TargetRepository interface {
	Create(ctx context.Context, target *domain.Target) error
	FindByID(ctx context.Context, id bson.ObjectID) (*domain.Target, error)
	FindAll(ctx context.Context, filter TargetFilter, limit, offset int64) ([]*domain.Target, int64, error)
	FindEnabled(ctx context.Context) ([]*domain.Target, error)
	Update(ctx context.Context, id bson.ObjectID, target *domain.Target) error
	Delete(ctx context.Context, id bson.ObjectID) error
}
```

Regras comportamentais:
- `Create`: gera `ID` se `IsZero()`, preenche `CreatedAt`/`UpdatedAt` em UTC, aplica defaults (`Method: "GET"`, `Timeout: 5s` se `<= 0`), mapeia erro de chave duplicada (`mongo.IsDuplicateKeyError`) para `domain.ErrAlreadyExists`.
- `FindByID`: retorna `domain.ErrNotFound` se documento não existir.
- `FindAll`: retorna slice (nunca erro se vazio; `[]*domain.Target{}`), `total` de documentos compatíveis com o filtro, ordenado por `created_at` descrescente (ou ordem consistente), respeitando `limit` e `offset`.
- `FindEnabled`: retorna apenas targets com `enabled: true` para execução do `checker`.
- `Update`: preserva `ID` e `CreatedAt`, atualiza `UpdatedAt = time.Now().UTC()`, retorna `domain.ErrNotFound` se não existir e `domain.ErrAlreadyExists` se colidir com outra URL.
- `Delete`: remove pelo `_id`; retorna `domain.ErrNotFound` se `DeletedCount == 0`.
- `WriteConcern`: `majority` para operações de mutação em `targets` (PACELC: Consistência).

#### `CheckLogRepository`

```go
type CheckLogFilter struct {
	TargetID   *bson.ObjectID
	IsUp       *bool
	StatusCode *int
	From       *time.Time
	To         *time.Time
	Search     string // match parcial case-insensitive em Name, URL ou Error
}

type CheckLogRepository interface {
	Create(ctx context.Context, checkLog *domain.CheckLog) error
	CreateBatch(ctx context.Context, logs []domain.CheckLog) error
	FindByID(ctx context.Context, id bson.ObjectID) (*domain.CheckLog, error)
	FindAll(ctx context.Context, filter CheckLogFilter, limit, offset int64) ([]domain.CheckLog, int64, error)
	FindByTargetID(ctx context.Context, targetID bson.ObjectID, limit, offset int64) ([]domain.CheckLog, int64, error)
}
```

Regras comportamentais:
- `Create` / `CreateBatch`: preenchem `ID` e `CheckedAt` (UTC) se ausentes; `CreateBatch` faz `InsertMany` (no-op se `len(logs) == 0`) com `WriteConcern: 1` (PACELC: Baixa Latência).
- `FindByID`: busca por `_id`; retorna `domain.ErrNotFound` se não encontrado.
- `FindAll`: aplica filtros (`target_id`, `is_up`, `status_code`, janela `from`/`to` sobre `checked_at`, busca textual), ordena por `checked_at: -1`, e retorna `(logs, total, nil)`.
- `FindByTargetID`: atalho filtrado por `target_id` ordenado por `checked_at: -1`.

### 3.3 Mocks / Fakes e Testes Unitários Offline

- `MockTargetRepository` e `MockCheckLogRepository` em `internal/database/mock.go` devem implementar **100%** das interfaces acima de forma thread-safe (`sync.RWMutex`), retornando cópias defensivas para evitar data races sob `go test -race`.
- **Deltas corrigidos do `mock.go` atual:**
  - `MockTargetRepository.Create` atualmente insere em `m.targets[idStr]` mas esquece de fazer `m.order = append(m.order, idStr)`, fazendo `FindAll` retornar vazio.
  - `MockTargetRepository.FindByID` atualmente faz `copied := target` (copia o ponteiro, não a struct).
  - Adicionar `Delete`, `FindEnabled`, filtro em `FindAll`, validação de unicidade de `URL` e `MockCheckLogRepository.FindByID` + filtros em `FindAll`.
- Testes unitários em `internal/database/repository_test.go` (ou equivalente) devem cobrir CRUD, paginação, filtros, erros (`ErrNotFound`, `ErrAlreadyExists`), concorrência (`-race`) e lote **sem exigir MongoDB ativo**.

---

## 4. Migração de Targets de Arquivo para MongoDB (`cmd/api` & `cmd/opspulse`)

### 4.1 Requisitos de Substituição

1. **`cmd/api/main.go` & `internal/api/`:**
   - Remover dependência de `cfg.Monitor.TargetURLs` carregado de arquivo em `TargetHandler` e `Server.StartMonitoring`.
   - Instanciar `MongoDBTargetRepository` e `MongoDBCheckLogRepository` a partir de `mongoClient.Database()` e injetá-los no `api.Server` (via interfaces `database.TargetRepository` e `database.CheckLogRepository`).
   - Cada ciclo de monitoramento busca os alvos ativos via `targetRepo.FindEnabled(ctx)`, executa as checagens via `checker`, publica evento SSE no `EventBroker` e persiste os resultados via `checkLogRepo.CreateBatch(ctx, logs)`.
2. **`cmd/opspulse/main.go` & `internal/discord/`:**
   - Inicializar `MongoClient` em `cmd/opspulse/main.go` com `defer mongoClient.Close(ctx)` (mesmo padrão de graceful shutdown de `cmd/api`).
   - Instanciar `MongoDBTargetRepository` e `MongoDBCheckLogRepository`.
   - `checker.StartMonitoring` passa a consultar `targetRepo.FindEnabled(ctx)` a cada ciclo e gravar `checkLogRepo.CreateBatch(ctx, logs)`.
   - `bot.Setup` (comando `/status` e botão `btn_recheck`) passa a consultar os alvos atualizados no `TargetRepository` via função/dependência injetada, eliminando a captura estática de `cfg.TargetURLs`.
3. **Configuração (`internal/config/monitor.go`):**
   - `LoadMonitorConfig` NÃO deve mais falhar caso `MONITOR_TARGETS_FILE` não exista (ou deve deixar de carregar alvos de arquivo no fluxo principal, mantendo apenas `MONITOR_INTERVAL`, `MONITOR_DEFAULT_TIMEOUT` e `AlertThreshold`).
   - `internal/file` (JSON/YAML loaders) pode ser mantido intacto para utilitários/testes existentes ou opcional de seed manual, desde que `cmd/api` e `cmd/opspulse` consumam exclusivamente o MongoDB em runtime.

---

## 5. Motor de Monitoramento (`internal/checker/`) — Jitter & Execução Rica

### 5.1 Jitter Estocástico ($\pm 20\%$)

- **Motivação:** Eliminar o efeito *Thundering Herd* (disparo simultâneo de todas as requisições no mesmo milissegundo em loops rígidos `time.NewTicker`).
- **Fórmula:**
  $$\text{Intervalo Efetivo} = \text{Intervalo Base} \pm \text{Random}(0, 0.2 \times \text{Intervalo Base})$$
  Equivalente a multiplicar `baseInterval` por um fator aleatório uniforme no intervalo $[0.8, 1.2]$ (variação máxima de $\pm 20\%$), garantindo piso mínimo seguro (ex: $\ge 100\text{ms}$ se `baseInterval > 0`).
- **Implementação:** Usar exclusivamente `math/rand/v2` da biblioteca padrão Go (sem dependências externas, conforme R4).
- **Aplicação:**
  1. **Agendamento independente por alvo (`RunTargetScheduler` / `StartMonitoring`):** Cada alvo habilitado possui sua própria goroutine agendada de forma independente com `time.NewTimer(JitterDuration(effectiveInterval))`, onde `effectiveInterval` é `target.Interval` (quando `> 0`) ou `MONITOR_INTERVAL` (quando `0`). Isso impede que todos os alvos disparem juntos e permite cadências únicas por alvo. Alterações de CRUD ou `triggerChan` disparam reconciliação imediata dos workers ativos.
  2. **Entre requisições por alvo (`CheckAll` / `CheckTargets`):** Nas checagens sob demanda em lote, aplicar pequeno offset/jitter aleatório por target antes do disparo HTTP (respeitando `ctx.Done()`), espalhando os pings ao longo da janela de execução.

### 5.2 Execução HTTP Rica no `checkURL`

- **Delta atual:** `internal/checker/checker.go` atualmente ignora `target.Method` (usa `http.MethodGet` fixo), `target.Headers`, `target.Body` e `target.ExpectedBodyMatch`.
- **Especificação:**
  - Utilizar `target.Method` normalizado em maiúsculas (fallback `http.MethodGet`).
  - Se `target.Body` não for vazio e o método aceitar payload (`POST`, `PUT`, `PATCH`), serializar como JSON e definir `Content-Type: application/json` (se não sobrescrito por `Headers`).
  - Aplicar `target.Headers` no `*http.Request`.
  - Aplicar `target.Timeout` (fallback `5 * time.Second` se `<= 0`).
  - Validar `StatusCode` contra `target.ExpectedStatus` (fallback `200..399` se vazio).
  - Se `target.ExpectedBodyMatch != ""`, ler corpo da resposta (com limite seguro, ex: `io.LimitReader` de 1MB) e verificar correspondência (substring ou regex válida, tratando erro de regex de forma explícita sem panic).
  - Mapear resultados preservando `TargetID` para gravação em `check_logs` e `CheckResult` para SSE/Discord.

---

## 6. API REST, SSE, CORS & OpenAPI (`internal/api/`, `internal/config/`)

### 6.1 Endpoints REST & Streaming

Base prefix: `/api/v1` (além das rotas de documentação `/docs` e `/docs/openapi.yaml`).

| Método | Rota | Descrição | Códigos HTTP |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/health` | Liveness check (`{"status":"OK"}`) | `200` |
| `GET` | `/api/v1/targets/status` | Executa checagem imediata nos targets habilitados e retorna `[]domain.CheckResult` | `200`, `500` |
| `GET` | `/api/v1/targets/events` | Stream SSE (`text/event-stream`), eventos `status`, `alert`, heartbeat `: ping\n\n` | `200`, `500` |
| `GET` | `/api/v1/targets` | Lista targets persistidos; query params: `enabled` (`true`/`false`), `tag`, `search`, `limit` (default `50`, max `200`), `offset` (default `0`) | `200`, `400`, `500` |
| `POST` | `/api/v1/targets` | Cria novo target no MongoDB; valida `name`, `url` (`http`/`https`), `method`, `timeout` | `201`, `400`, `409`, `500` |
| `GET` | `/api/v1/targets/{id}` | Busca target por `ObjectID` | `200`, `400`, `404`, `500` |
| `PUT` | `/api/v1/targets/{id}` | Atualiza target existente por `ObjectID` | `200`, `400`, `404`, `409`, `500` |
| `PATCH` | `/api/v1/targets/{id}` | Atualização parcial (ex: alternar `enabled`) | `200`, `400`, `404`, `409`, `500` |
| `DELETE` | `/api/v1/targets/{id}` | Remove target por `ObjectID` | `204` (ou `200`), `400`, `404`, `500` |
| `GET` | `/api/v1/check-logs` | Lista `check_logs` com paginação e filtros: `target_id`, `is_up`, `status_code`, `from` (RFC3339), `to` (RFC3339), `search`, `limit` (default `50`, max `500`), `offset` (default `0`) | `200`, `400`, `500` |
| `GET` | `/api/v1/check-logs/{id}` | Busca um `check_log` por `ObjectID` | `200`, `400`, `404`, `500` |
| `GET` | `/docs` | Interface interativa Scalar API Reference | `200` |
| `GET` | `/docs/openapi.yaml` | Especificação OpenAPI 3.1 estática/gerada | `200` |

#### Formato Padronizado de Resposta Paginada (Listagens)

Para `GET /api/v1/targets` e `GET /api/v1/check-logs`:

```json
{
  "data": [],
  "pagination": {
    "total": 120,
    "limit": 50,
    "offset": 0
  }
}
```

#### Formato de Erro HTTP (`SendError`)

```json
{
  "error": "mensagem descritiva do erro"
}
```

### 6.2 CORS Configurável via `.env`

- **Variável de ambiente:** `CORS_ALLOWED_ORIGINS` (lista separada por vírgulas, ex: `http://localhost:5173,http://localhost:5174` em `.env.example`).
- **`ServerConfig` (`internal/config/server.go`):** adicionar campo `AllowedOrigins []string` carregado a partir de `CORS_ALLOWED_ORIGINS` (trim de espaços, ignorar itens vazios; fallback seguro local para desenvolvimento como `http://localhost:5173,http://localhost:5174`).
- **Comportamento do middleware (`internal/api/middleware.go`) e `PrepareSSE` (`internal/api/responses.go`):**
  - **Remover** `w.Header().Set("Access-Control-Allow-Origin", "*")` hardcoded de `corsMiddleware` e `PrepareSSE`.
  - Inspecionar o header `Origin` da requisição: se estiver na lista `AllowedOrigins` (ou se `"*"` for explicitamente configurado na variável de ambiente), definir `Access-Control-Allow-Origin: <origin>` e `Vary: Origin`.
  - Definir `Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE, OPTIONS` e `Access-Control-Allow-Headers: Content-Type, Authorization`.
  - Responder `204 No Content` / `200 OK` imediatamente em `OPTIONS` preflight quando a origem é permitida (ou rejeitar preflight não autorizado sem liberar CORS).

### 6.3 OpenAPI & Correção de Imports

- Substituir `github.com/go-chi/chi/middleware` por `github.com/go-chi/chi/v5/middleware` em `internal/api/server.go`.
- Após implementar as novas rotas em M3, regenerar `docs/openapi.yaml` executando `make api-docs` (`apispec -o docs/openapi.yaml`).

---

## 7. Frontend Web (`web/opspulse-frontend/`)

### 7.1 Cliente HTTP (`axios`) & Variáveis de Ambiente

- Instalar `axios` em `web/opspulse-frontend/package.json`.
- Configurar variável de ambiente `VITE_API_BASE_URL` (documentada em `.env.example` / `web/opspulse-frontend/.env.example`, ex: `http://localhost:3333` ou vazio para usar o proxy `/api` do Vite).
- Criar instância centralizada em `src/lib/api.ts`:
  ```ts
  const baseURL = import.meta.env.VITE_API_BASE_URL ?? "";
  export const api = axios.create({ baseURL });
  ```
- Atualizar `useSSE`: construir a URL do `EventSource` a partir de `import.meta.env.VITE_API_BASE_URL` (ex: `${baseURL}/api/v1/targets/events`) e garantir retorno de cleanup `() => es.close()` no `useEffect`.
- **Proibido hardcode** de `http://localhost:3333` ou listas estáticas de targets no frontend.

### 7.2 Modal de Criação & Atualização de Target

- Componente modal acessível no Dashboard principal permitindo **Criar** (`POST /api/v1/targets`), **Editar** (`PUT /api/v1/targets/{id}`), **Alternar Enabled** (`PATCH` ou `PUT`) e **Excluir** (`DELETE /api/v1/targets/{id}` com confirmação).
- Campos suportados no formulário alinhados ao domínio Go:
  - `name` (obrigatório)
  - `url` (obrigatório, validação `http://` ou `https://`)
  - `method` (`GET`, `POST`, `PUT`, `HEAD`, `DELETE`)
  - `timeout` (exibido/editado em ms no formulário e convertido para nanosegundos `time.Duration`; `0` usa `MONITOR_DEFAULT_TIMEOUT`)
  - `interval` (exibido/editado em segundos no formulário e convertido para nanosegundos `time.Duration`; vazio/`0` herda `MONITOR_INTERVAL`)
  - `expected_status` (ex: input separado por vírgula `200, 201, 204` -> `number[]`)
  - `headers` (pares chave/valor editáveis ou JSON válido -> `Record<string, string>`)
  - `body` (JSON opcional quando método for `POST`/`PUT`)
  - `expected_body_match` (string opcional)
  - `tags` (tags separadas por vírgula -> `string[]`)
  - `enabled` (toggle booleano)
- Tratamento explícito de estados de loading, erro de validação/servidor (`400`/`409`/`500`) e atualização imediata da lista após persistir.

### 7.3 Layout Principal em Cards (Substituição da Tabela)

- Substituir `TargetTable.tsx` por layout responsivo em **grid de Cards** (`TargetCards.tsx`) mantendo a identidade visual dark (`slate-950`/`slate-900`, Tailwind v4, `lucide-react`).
- Combinar os dados cadastrados no MongoDB (`GET /api/v1/targets`) com as atualizações em tempo real recebidas via SSE (`useSSE`), exibindo em cada card:
  - Status ao vivo (`UP` / `DOWN` / `Pausado` se `enabled === false` / `Aguardando`)
  - Nome, URL clicável/copiável, método HTTP e badges de `tags`
  - Código HTTP retornado e status esperados configurados
  - Latência formatada em ms (`(Latency / 1_000_000).toFixed(2) ms`), timeout e intervalo configurados
  - Diagnóstico / mensagem de erro (quando houver) e indicador visual de último check
  - Botões de ação no card: Editar (abre modal preenchido), Ativar/Desativar, Ver Checklogs filtrados por este target, Excluir.
- **Filtros de disponibilidade persistidos no `localStorage`:** Barra de filtros por disponibilidade (`Todos`, `UP`, `DOWN`, `Pausado`, `Aguardando`) combinada com a busca textual, salvando a preferência do usuário em `localStorage` (`opspulse.dashboard.availabilityFilter`) para persistir entre recarregamentos.

### 7.4 Página de Checklogs (`CheckLogsPage.tsx`) com Filtros & Insights

- Navegação simples no `Header` / `App.tsx` entre **Dashboard** e **Checklogs** (sem adicionar biblioteca de roteamento pesada se estado/URL query/hash nativo resolver com simplicidade, conforme R4).
- **Consumo de dados:** `GET /api/v1/check-logs` com query params (`target_id`, `is_up`, `status_code`, `from`, `to`, `search`, `limit`, `offset`) e paginação.
- **Filtros interativos:**
  - Serviço / Target (`target_id` populado via `GET /api/v1/targets`)
  - Status (`Todos`, `UP`, `DOWN`)
  - Código HTTP (`status_code`, ex: `200`, `404`, `429`, `500`, `0`)
  - Período (`Última 1h`, `Últimas 24h`, `Últimos 7 dias`, `Últimos 30 dias` ou intervalo customizado `from`/`to`)
  - Busca textual (`search` por nome, URL ou mensagem de erro)
- **Insights calculados a partir dos campos de `check_logs`** (computados com funções puras TypeScript, zero libs pesadas):
  1. **Uptime Geral (%):** `(total_up / total_checks) * 100`
  2. **Latência Média, Mínima, Máxima e P95 (ms):** calculadas sobre `latency / 1_000_000`
  3. **Contagem de Falhas & Taxa de Erro (%):** total de `is_up === false`
  4. **Distribuição por Código HTTP:** agrupamento por `status_code` (2xx, 3xx, 4xx, 5xx, Timeout/Erro de Rede `0`)
  5. **Alvo Mais Rápido (Média), Top Alvos Mais Lentos & Alvos com Mais Falhas:** destaque para o alvo com menor latência média válida (`fastestTarget`) e rankings agregados por `target_id`/`url` mostrando latência média e quantidade de quedas no conjunto filtrado.
  6. **Lista / Timeline detalhada dos Checklogs:** timestamp formatado, serviço/URL, badge UP/DOWN, status code, latência em ms e erro completo.

---

# PARTE II — ROADMAP FUTURO (PÓS-R4)

> Os requisitos abaixo foram consolidados integralmente da documentação anterior (`docs/steps/*`) e constituem evoluções planejadas **fora** do escopo imediato R1–R4.

## 8. Autenticação & Autorização Sem Senha (API v2)

- **Coleção `users` (MongoDB):**
  - `_id` (`bson.ObjectID`), `github_id` (`string`, índice único), `username` (`string`), `email` (`string`), `avatar_url` (`string`), `api_keys` (`[]APIKey`: hash SHA-256, nome, `created_at`, `last_used_at`), `created_at`, `updated_at`.
  - Associação de `targets.user_id` (`bson.ObjectID`, índice composto com `url`).
- **Para Humanos (Dashboard React) — GitHub OAuth 2.0:**
  1. `GET /api/v1/auth/github` redireciona para consentimento OAuth2 do GitHub (`oauth2.Config`).
  2. `GET /api/v1/auth/github/callback?code=...` valida código, cria/atualiza usuário na coleção `users`.
  3. Emite **JWT de Sessão** em **Cookie HTTP-Only** (`SameSite=Lax`, `Secure` em produção), sem expor token no `localStorage` (mitigando XSS).
  4. CORS com `AllowCredentials: true` restrito ao domínio oficial.
- **Para Automações (CLI, Bot Discord, Scripts) — Personal Access Tokens (PAT):**
  - Geração de chaves com prefixo (ex: `op_live_...`), armazenando apenas `SHA-256(token)` no banco.
  - Autenticação via header `Authorization: Bearer op_live_...`.

## 9. Hardening OWASP Avançado & Proteções em Nuvem

- **Headers HTTP OWASP (Backend Go):**
  - `X-Content-Type-Options: nosniff`
  - `X-Frame-Options: DENY`
  - `Strict-Transport-Security: max-age=63072000; includeSubDomains; preload`
  - `Content-Security-Policy: default-src 'self'` (adaptado para `/docs`)
  - `Referrer-Policy: strict-origin-when-cross-origin`
  - `Permissions-Policy: camera=(), microphone=(), geolocation=()`
  - Limitação de payload via `http.MaxBytesReader` contra DoS por body gigante.
- **Camadas Delegadas à Nuvem (Terraform/Edge):**
  - **WAF:** filtragem de SQLi/NoSQL Injection na borda (Cloudflare / AWS WAF).
  - **Rate Limiting Distribuído & Anti-DDoS:** na CDN/Edge.
  - **Isolamento de Rede:** subnets privadas para MongoDB, Security Groups restritivos e TLS automático.

## 10. Resiliência Avançada do Motor de Monitoramento (`internal/checker/`)

Evolução sobre o Jitter $\pm 20\%$ (já no escopo R2):
- **Backoff Exponencial:** em falhas consecutivas, dobrar o tempo de espera ($2 \times \text{intervalo}$), limitado ao teto máximo de **5 minutos**.
- **Tratamento de HTTP `429 Too Many Requests`:**
  - Respeitar header `Retry-After` quando presente.
  - Caso ausente, aplicar cool-down fixo de **60 segundos** para evitar banimento de IP por WAFs (Cloudflare, AWS WAF, Nginx).
- **Circuit Breaker Minimalista por Alvo:**
  - Struct simples sem libs externas: `ConsecutiveFailures int`, `NextCheckAt time.Time`, `State (Closed, Open, HalfOpen)`.
  - `CLOSED`: monitora normalmente com jitter.
  - `OPEN`: após $N$ falhas consecutivas (ex: 3 a 5), entra em quarentena por 2 a 5 minutos.
  - `HALF-OPEN`: ao expirar a quarentena, envia uma única *canary probe* (sucesso $\to$ `CLOSED`; falha $\to$ reinicia `OPEN`).

## 11. Incidentes, SLA, Multi-Notificadores & Observabilidade Avançada

- **Coleção `incidents` (`internal/domain/incidents.go`, `migrations/000003`):**
  - Campos: `ID`, `TargetID`, `URL`, `Status` (`OPEN` | `RESOLVED`), `StartedAt`, `ResolvedAt`, `DurationSeconds`, `RootCause`.
  - Anti-flapping usando `AlertThreshold` e cálculo automático de SLA/downtime.
- **Checkers & Notificadores Extensíveis:**
  - `TCPChecker`, `SSLChecker` (SSL Watchdog para vencimento de certificados TLS).
  - Multi-Notificadores (Slack Block Kit, Telegram, Email SMTP/SES, Microsoft Teams) via Factory/Composite `Notifier`.
- **Microserviços & Observabilidade:**
  - Filas assíncronas (NATS/RabbitMQ/Redis Streams) e exportação de métricas Prometheus/Grafana.

## 12. Infraestrutura Cloud & CI/CD (AWS EC2 + Terraform + Ansible + GitHub Actions)

- **Fase 1 (AWS Conta & IAM):** MFA no Root, *AWS Budgets* (> $0.01) e Free Tier alerts, usuário IAM programático (`AmazonEC2FullAccess`, `AmazonVPCFullAccess`) + AWS CLI (`aws sts get-caller-identity`).
- **Fase 2 (Terraform em `deployments/terraform/`):** chave `ed25519` (`aws_key_pair`), `aws_security_group` (SSH porta 22 restrita ao IP do operador, outbound `0.0.0.0/0`), `aws_instance` `t2.micro`/`t3.micro` Ubuntu 24.04 LTS, `outputs.tf` com IP público.
- **Fase 3 (Ansible em `build/ansible/`):** `inventory.ini` + playbook `setup-ec2.yml` (apt upgrade, `curl`, `git`, `htop`, `ca-certificates`, Docker Engine + Compose plugin, usuário `ubuntu` no grupo `docker`, `/opt/opspulse`, `ufw`).
- **Fase 4 (GitHub Actions CD):**
  - Secrets (`SSH_PRIVATE_KEY`, `EC2_HOST`, `EC2_USER`, `DISCORD_TOKEN`, `DISCORD_CHANNEL_ID`) e environment `production` com aprovação manual (*Required reviewers*).
  - Pipeline em 3 jobs: `test_and_lint` (`go test -v -race ./...` + `golangci-lint`) $\to$ `build_and_push` (`ghcr.io`) $\to$ `deploy` (SSH na EC2, pull da imagem, subida com `restart: unless-stopped`).
- **Fase 5 (Smoke Test em Produção):** `ssh` na EC2, `docker ps`, `docker logs -f opspulse` e `/status` no Discord.
