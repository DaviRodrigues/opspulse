---
date: 2026-09-28
project: go-project
topic: docker-dev-prod-and-air
---

**Where they got stuck:**
- Forma `CMD ["air ${TARGET_AIR}"]` (exec form) no Dockerfile não expandia o argumento de build/variável de ambiente, quebrando o live reload do Air.
- Mapeamento de volumes relativos ao mover o `docker-compose.yml` para o diretório `deployments/`, resultando em diretórios `/app` vazios ou fora de contexto.

**What unlocked it:**
- Separação clara de concerns entre `Dockerfile.dev` (com Air e ferramentas de desenvolvimento) e `Dockerfile.prod` (multi-stage com imagem final scratch/alpine enxuta).
- Passagem do arquivo de configuração do Air via parâmetro de comando no docker-compose (`command: air -c .air.api.toml`) e ajuste do bind mount para `../:/app`.

**Demonstrated mastery of:**
- Criação de Dockerfiles multi-stage para compilação estática de binários Go com flags `-ldflags="-s -w"`.
- Otimização de cache de camadas do Docker e persistência do Go module cache com volumes nomeados (`go-pkg-mod`).
- Orquestração de múltiplos serviços (API, Monitor, Frontend) via Docker Compose com profiles (`dev` e `prod`).

**Left open:**
- Configuração de build e pipeline de deploy CI/CD para geração automática de imagens de produção.
