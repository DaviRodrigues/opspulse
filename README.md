# OpsPulse

**OpsPulse** é um serviço de monitoramento e ChatOps desenvolvido em **Go**, projetado para checar a disponibilidade de serviços e integrar notificações automáticas com o **Discord**, contando com esteira completa de **DevOps (Docker, GitHub Actions e Terraform)**.

---

## Documentação & Planejamento

- **[Plano de Projeto &amp; Roadmap por Fases](docs/PROJECT_PLAN.md)**: Detalhamento do MVP, arquitetura e conceitos teóricos divididos em 6 etapas incrementais.
- **[Banco de Ideias de Projetos Futuros](docs/ideas/other_projects_ideas.md)**: Ideias salvas para projetos posteriores (em Go ou Python).

---

## Tecnologias e Ferramentas

- **Linguagem:** Go (Golang)
- **Comunicação / ChatOps:** Discord API (`discordgo`)
- **Containerização:** Docker (Multi-stage build)
- **Integração Contínua (CI):** GitHub Actions
- **Infraestrutura como Código (IaC):** Terraform

---

## Status do Projeto

- [X] **Fase 1:** Setup do Módulo & Health Checker Básico _(Próximo passo)_
- [X] **Fase 2:** Concorrência com Goroutines & Tickers
- [ ] **Fase 3:** Integração com Discord Bot
- [ ] **Fase 4:** Dockerização & Docker Compose
- [ ] **Fase 5:** CI/CD com GitHub Actions
- [ ] **Fase 6:** Infraestrutura com Terraform
