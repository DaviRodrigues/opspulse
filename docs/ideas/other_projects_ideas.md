# 💡 Banco de Ideias para Projetos Futuros (MVPs & Portfólio)

> Repositório de ideias com escopo enxuto (foco em MVP viável por um único desenvolvedor), priorizando validação prática de arquitetura, padrões de projeto e ferramentas de mercado.

---

## 💻 1. Desenvolvimento Web / SaaS (Dev)

### 🚀 Ideia 1: ManagerEasy — Hub de Gestão & Financeiro Simplificado (PF/PJ)
* **Objetivo:** Resolver a sobrecarga de ferramentas complexas (Notion/Excel) com um livro-caixa objetivo para autônomos e pequenos negócios.
* **Stack Sugerida:** PHP (Laravel / Symfony) ou TypeScript (NestJS / Next.js) + PostgreSQL.
* **Escopo do MVP:**
  - **Autenticação & Multi-tenancy simples:** Separação de contas PF e PJ.
  - **Livro-caixa com Categorias:** Lançamento de Receitas/Despesas com status (Pago/Pendente) e anexo de comprovantes.
  - **Dashboard Financeiro:** Gráficos de saldo mensal, fluxo de caixa e exportação (PDF/CSV).
  - **Alertas de Vencimento:** Notificação via e-mail ou webhook sobre contas a vencer.

---

### 🔄 Ideia 2: WebhookPulse — Plataforma de Roteamento & Retentativa de Webhooks
* **Objetivo:** Garantir a entrega resiliente de webhooks críticos (ex: gateways de pagamento como Stripe e Mercado Pago).
* **Stack Sugerida:** TypeScript (Fastify / Node.js) ou PHP (Octane) + Redis.
* **Escopo do MVP:**
  - **Endpoint Receptor Central:** Recebe o payload do webhook e enfileira no Redis.
  - **Worker de Despacho:** Encaminha a requisição para a URL de destino com política de *exponential backoff* em caso de falha (5xx).
  - **Painel de Auditoria:** Interface minimalista listando histórico de webhooks recebidos, logs de tentativa e botão de reenvio manual.

---

## ⚙️ 2. DevOps, Infraestrutura & Ferramental (DevOps)

### 🦀 Ideia 3: RustLoad — CLI de Testes de Carga & Benchmark HTTP (Rust)
* **Objetivo:** Aprender Rust criando uma ferramenta de linha de comando de altíssima performance e baixo consumo de memória.
* **Stack Sugerida:** Rust (`tokio` para assincronismo, `reqwest`/`hyper` para HTTP, `clap` para argumentos CLI).
* **Escopo do MVP:**
  - **Comando CLI:** `rustload -u <URL> -c <concorrencia> -d <duracao>` (ex: 100 conexões simultâneas por 30s).
  - **Métricas de Performance:** Cálculo em tempo real de latência (p50, p95, p99), RPS (requisições por segundo) e distribuição de status HTTP.
  - **Relatório no Terminal:** Sumário visual formatado no console com tabela de resultados.

---

### 🛡️ Ideia 4: DriftWatch — Detector de Desvios de Boas Práticas em Contêineres Docker (Go / Python)
* **Objetivo:** Auditar ambientes locais e de homologação contra más práticas de segurança e alocação de recursos em contêineres.
* **Stack Sugerida:** Go (Docker SDK) ou Python.
* **Escopo do MVP:**
  - **Inspeção de Contêineres:** Scanner que consulta a Docker API local.
  - **Regras de Conformidade:** Identifica contêineres sem limites de memória/CPU (`limits`), sem política de reinicialização (`restart_policy`) ou com portas expostas diretamente em `0.0.0.0`.
  - **Exportação de Relatório:** Geração de sumário em Markdown e disparo de alerta em canal do Discord/Slack.