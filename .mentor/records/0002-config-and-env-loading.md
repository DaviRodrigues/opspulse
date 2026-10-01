---
date: 2026-09-25
project: go-project
topic: config-and-env-loading
---

**Where they got stuck:**
- Lógica de fallback em `LoadVariable`, que retornava erro mesmo quando havia valor default fornecido caso a variável de ambiente não estivesse setada.
- Typo no arquivo `.env` (ex: `""./targets.yaml""` com aspas extras) sendo silenciado porque o retorno de `godotenv.Load` era ignorado com `_`.

**What unlocked it:**
- Checagem explícita de `if fallback != ""` antes de emitir erro de variável ausente.
- Adição de logs estruturados durante o carregamento das variáveis em `NewEnvFile` e divisão de responsabilidades por domínio (`SERVER_`, `MONITOR_`, `DISCORD_`).

**Demonstrated mastery of:**
- Princípios da 12-Factor App para gerenciamento de configurações via variáveis de ambiente.
- Estruturação modular de configurações com sub-structs tipadas (`AppConfig`, `ServerConfig`, `MonitorConfig`, `LogConfig`, `DiscordConfig`).
- Validação fail-fast com agregação de múltiplos erros via `errors.Join`.
- Testes unitários com `t.Setenv` cobrindo cenários com e sem variáveis presentes.

**Left open:**
- Recarregar variáveis de ambiente em tempo de execução via sinal `SIGHUP` ou endpoint de reload.
