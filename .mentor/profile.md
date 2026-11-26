# Mentor Profile

Histórico de evolução, tópicos dominados, pontos de bloqueio e diagnóstico individual de engenharia no projeto **OpsPulse**.

---

## Diagnóstico do Desenvolvedor (Análise Individual)

### Pontos Acertados (Forças & Destaques de Engenharia)
1. **Intuição Arquitetural e Adoção Proativa de Padrões:**
   - Capacidade natural de identificar quando o código está rígido e propor refatorações elegantes (ex: transição para o padrão **Strategy** com Higher-Order Functions no carregamento de arquivos e **Actor Model** sem mutex para o broker SSE).
   - Preocupação constante com escalabilidade e desacoplamento, separando claramente domínios de configuração (12-Factor App) e camadas no frontend (React + Tailwind modular).
2. **Mentalidade de "Mão na Massa" e Aprendizado Ativo:**
   - Digita e implementa todas as soluções, buscando compreender a raiz do problema em vez de aceitar correções superficiais ou mágicas.
   - Excelente evolução em testes de integração simulados (`httptest.Server`, `t.Setenv`, testes de concorrência com canais e convenção `testdata/`).
3. **Atenção à Observabilidade e Produção:**
   - Foco precoce em logs estruturados com `slog`, metadados contextuais (`app`, `env`) e warnings de segurança para ambientes produtivos.
4. **Centralização e Eliminação de Código Duplicado:**
   - Iniciativa de unificar todas as opções de fallback em uma struct imutável de pacote (`defaultFallback`), eliminando magic strings e hardcoding.

---

### Erros & Vícios de Raciocínio (Onde Escorregou)
1. **Suposições de Runtime vs. Semântica de Tipos (Go):**
   - *Cópia por Valor no `range`:* Tentativa inicial de mutação de structs dentro de loops `for _, t := range targets` sem considerar que `t` é uma cópia alocada na stack.
   - *Reflexão em Interfaces:* Supor que a interface `error` serializaria nativamente para JSON com mensagem legível, esquecendo que campos internos não exportados viram `{}` via `encoding/json`.
2. **Ciclo de Vida e Efeitos Colaterais Assíncronos:**
   - *Go (Orquestração):* Bloqueio acidental de fluxo ao invocar `server.Start()` síncrono antes do monitoramento em background.
   - *React (Estado Efêmero):* Declarar instâncias de conexão (`let eventSourceRef`) no escopo local do hook, perdendo a referência a cada re-renderização do componente em vez de usar `useRef`.
3. **Engessamento e Acoplamento de Testes Unitários:**
   - Acoplar testes unitários a arquivos de documentação pública (`.env.example`) ou testar cenários de fallback passando arquivos de fixture populados (`envTest`), mascarando o comportamento real de fallbacks em tempo de execução.
4. **Erros Silenciosos de Configuração:**
   - Tratamento com `_ = godotenv.Load()` que engolia erros de sintaxe no `.env` e retorno prematuro de erro em variáveis opcionais não declaradas.

---

### Plano de Melhorias & Diretrizes para o Próximo Nível
1. **Design de Ciclo de Vida Antes do Código:**
   - Antes de iniciar um recurso assíncrono ou com estado, desenhar mentalmente: *"Quem instancia? Quem consome? Quem encerra? O que acontece se a rede cair ou o consumidor for lento?"*.
2. **Separação Clara de Testes (Carga vs Fallback vs Override):**
   - Manter testes de carga completa isolados em `testdata/`, testes de fallback isolados (sem arquivos de entrada e com variáveis limpas) e testes de override via `t.Setenv`.
3. **Resiliência e Engenharia de Caos:**
   - Implementar padrões de produção como *Exponential Backoff com Jitter*, *Circuit Breakers* e tratamento de *Slow Consumers* (Backpressure).
4. **Transição para Infraestrutura como Código (Terraform/Cloud):**
   - Aproveitar o rigor aprendido na separação de configurações e ambientes para modelar infraestrutura imutável, declarativa e modularizada.

---

## Mastered

Habilidades e conceitos consolidados, demonstrados com autonomia e explicados na prática:

- **2026-09-24** — `concurrency-and-fanout` ([0001-concurrency-and-fanout.md](file:///home/smu/repositories-git/go-project/.mentor/records/0001-concurrency-and-fanout.md))
  - Padrão Fan-out / Fan-in com goroutines, buffered channels e `sync.WaitGroup` / `wg.Go(...)`.
  - Checagem concorrente com `context.WithTimeout` e testes isolados com `httptest.Server`.
- **2026-09-25** — `config-and-env-loading` ([0002-config-and-env-loading.md](file:///home/smu/repositories-git/go-project/.mentor/records/0002-config-and-env-loading.md))
  - 12-Factor App config, separação por domínios de configuração (`ServerConfig`, `MonitorConfig`, `LogConfig`, `DiscordConfig`).
  - Fallback handling robusto e validação unificada de erros com `errors.Join`.
- **2026-09-26** — `sse-broker-and-http-lifecycle` ([0003-sse-broker-and-http-lifecycle.md](file:///home/smu/repositories-git/go-project/.mentor/records/0003-sse-broker-and-http-lifecycle.md))
  - Arquitetura de Pub/Sub com Actor Model e canais em Go (zero lock contention).
  - Ciclo de vida de conexões persistentes Server-Sent Events (SSE) e formatação W3C.
- **2026-09-28** — `docker-dev-prod-and-air` ([0004-docker-dev-prod-and-air.md](file:///home/smu/repositories-git/go-project/.mentor/records/0004-docker-dev-prod-and-air.md))
  - Docker multi-stage builds (`Dockerfile.dev` com Air e `Dockerfile.prod` otimizado).
  - Gestão de cache de dependências Go (`go-pkg-mod`) e compose profiles.
- **2026-09-29** — `json-error-serialization` ([0005-json-error-serialization.md](file:///home/smu/repositories-git/go-project/.mentor/records/0005-json-error-serialization.md))
  - Compreensão de reflexão e regras de exportação do pacote `encoding/json` com interfaces `error`.
  - Modelagem limpa de DTOs e eventos serializáveis para clientes web.
- **2026-09-30** — `react-hook-sse-lifecycle` ([0006-react-hook-sse-lifecycle.md](file:///home/smu/repositories-git/go-project/.mentor/records/0006-react-hook-sse-lifecycle.md))
  - Criação de Custom Hooks no React 19 + TypeScript com `useRef` para instâncias mutáveis de conexão.
  - Integração modular de frontend com Tailwind CSS v4 e Lucide Icons.
- **2026-10-01** — `strategy-pattern-file-loaders` ([0007-strategy-pattern-file-loaders.md](file:///home/smu/repositories-git/go-project/.mentor/records/0007-strategy-pattern-file-loaders.md))
  - Padrão Strategy com Higher-Order Functions para decodificadores flexíveis (JSON/YAML).
  - Desacoplamento de leitura I/O e parsing através da interface `TargetLoader` e Factory pattern.
- **2026-10-01** — `config-fallback-centralization-and-test-isolation` ([0008-config-fallback-centralization-and-test-isolation.md](file:///home/smu/repositories-git/go-project/.mentor/records/0008-config-fallback-centralization-and-test-isolation.md))
  - Centralização de defaults/fallbacks de configuração com struct privada imutável (`defaultFallback`).
  - Isolamento de fixtures de teste na convenção `testdata/` e distinção entre testes de carga completa vs fallback real.
  - Automação de tarefas de desenvolvimento com `Makefile` e scripts seguros.

---

## Stuck

Pontos de atenção recorrentes ou tópicos com oportunidade de aprofundamento futuro:

- **Estratégias de Resiliência e Backpressure** (1 ocorrência — [0001](file:///home/smu/repositories-git/go-project/.mentor/records/0001-concurrency-and-fanout.md), [0003](file:///home/smu/repositories-git/go-project/.mentor/records/0003-sse-broker-and-http-lifecycle.md))
  - *Contexto:* Tratamento de slow consumers no canal SSE e implementação de retry com backoff exponencial / anti-flapping no monitor.
  - *Ação recomendada:* Projetar mecanismo de drop-oldest ou buffer dinâmico para streams em alta frequência.
- **Reconexão Automática e Tratamento de Desconexão no Frontend** (1 ocorrência — [0006](file:///home/smu/repositories-git/go-project/.mentor/records/0006-react-hook-sse-lifecycle.md))
  - *Contexto:* Quando o backend é reiniciado ou a rede oscila, o `EventSource` nativo tenta reconectar, mas um controle fino com backoff customizado na UI garante melhor experiência.
  - *Ação recomendada:* Implementar indicador de status de conexão na UI (badge visual: Conectado / Reconectando / Offline) com tentativas escalonadas.

