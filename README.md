# 📡 OpsPulse — ChatOps & Infra Health Check Bot

**OpsPulse** é uma solução de monitoramento de infraestrutura e ChatOps desenvolvida em **Go**, projetada para verificar a saúde de serviços concorrentemente e despachar alertas e relatórios interativos para o **Discord**, contando com uma esteira completa de automação **DevOps (Docker, GitHub Actions e Terraform)**.

---

## 📚 Documentação do Projeto

- 📖 **[Plano de Projeto & Roadmap por Fases (docs/PROJECT_PLAN.md)](docs/PROJECT_PLAN.md)**: Detalhamento da arquitetura, status do MVP e pacotes de evolução pós-MVP.
- 💡 **[Banco de Ideias de Projetos Futuros (docs/ideas/other_projects_ideas.md)](docs/ideas/other_projects_ideas.md)**: Ideias salvas para implementação futura em Go ou Python.

---

## 🛠️ Tecnologias & Padrões

- **Linguagem:** Go (Golang 1.24+)
- **Concorrência:** Goroutines, Channels com buffer, `sync.WaitGroup`, `time.Ticker` e Non-blocking Select.
- **Comunicação / ChatOps:** Discord API (`discordgo`), Embeds ricos, Slash Commands (`/status`) e Botões Interativos (🔄 "Checar Novamente").
- **Observabilidade:** Structured Logging com `log/slog` nativo e rotação de logs diários.
- **Containerização:** Docker (Multi-stage build gerando imagem < 20MB) e Docker Compose.
- **CI/CD:** GitHub Actions com testes automatizados, race detector, linters e releases multi-plataforma.
- **Infraestrutura como Código (IaC):** Terraform para provisionamento de nuvem.

---

## 🚀 Status do Projeto

### 🎯 Fase MVP

- [x] **Fase 1:** Setup do Módulo, Structs de Dados e Health Checker Básico
- [x] **Fase 2:** Concorrência, Goroutines, Channels, Tickers e Graceful Shutdown
- [x] **Fase 3:** Configuração com Fallbacks, Structured Logging (`slog`) e Discord Bot Interativo
- [x] **Fase 4:** Dockerfile Multi-stage otimizado e Docker Compose com volumes
- [x] **Fase 5:** CI/CD Pipeline no GitHub Actions (Testes, Linters e Releases de Binários)
- [ ] **Fase 6:** Infraestrutura como Código (IaC) com Terraform _(Em andamento)_

---

## 🔮 Roadmap Pós-MVP (Evoluções em Pacotes)

As futuras atualizações estão estruturadas em pacotes modulares independentes:

- **📦 Pacote 1 — Targets Ricos (JSON/YAML):** Suporte a arquivos de configuração com validação de status HTTP customizado, headers personalizados (`Authorization`), métodos `POST`/`PUT` e validação de body/payload (regex e JSON match).
- **📦 Pacote 2 — Plataforma Multi-Notificadores:** Adição de **Slack** (Block Kit), **Telegram**, **Email** (SMTP/SES) e **Microsoft Teams** através do padrão _Factory/Composite Notifier_.
- **📦 Pacote 3 — Persistência NoSQL & Métricas de SLA:** Armazenamento de histórico de quedas e tempos de resposta em banco NoSQL (MongoDB, Redis ou SQLite), cálculo de uptime/SLA e threshold anti-flapping (evita falsos positivos).
- **📦 Pacote 4 — API REST & Dashboard Web:** Endpoints REST para cadastro de URLs em runtime sem reiniciar o container, e dashboard web com gráficos de latência em tempo real.
- **📦 Pacote 5 — Arquitetura de Microserviços & Mensageria:** Desacoplamento entre o motor de checagem e os notificadores usando filas assíncronas (NATS, RabbitMQ ou Redis Streams).
- **📦 Pacote 6 — Observabilidade Avançada & SSL Watchdog:** Exportador de métricas para **Prometheus / Grafana** e alertas antecipados de expiração de certificados SSL/TLS (HTTPS).

---

## 💻 Como Rodar Localmente

### Pré-requisitos

- Go 1.24+ ou Docker

### 1. Clonar e Configurar Variáveis

```bash
cp .env.example .env
# Edite o .env com seu DISCORD_TOKEN, DISCORD_CHANNEL_ID e TARGET_URLS
```

### 2. Rodar com Go

```bash
go run ./cmd/opspulse
```

### 3. Rodar com Docker Compose

```bash
docker compose up -d
```

### 4. Rodar Suíte de Testes

```bash
go test -v -race ./...
```
