---
date: 2026-10-01
project: go-project
topic: config-fallback-centralization-and-test-isolation
---

**Where they got stuck:**
- Acoplamento de artefatos de documentação (`.env.example`) diretamente em testes unitários, tornando os testes frágeis a alterações em valores de exemplo.
- Execução de teste de fallback (`TestServerConfig_Fallback`) passando um arquivo com valores pré-configurados (`envTest`), fazendo com que o teste recebesse o valor explícito em vez de exercitar o fallback real.
- Dispersão de valores padrão (magic strings) em múltiplos arquivos de sub-carregadores (`app.go`, `server.go`, `monitor.go`, `log.go`).

**What unlocked it:**
- Adoção da convenção oficial `testdata/` do Go para manter fixtures de teste (`config_valid.env` e `target_test.json`) 100% isoladas da raiz do projeto.
- Centralização de todos os valores de fallback em uma struct unificada privada (`var defaultFallback = Config{...}`), eliminando duplicidade e facilitando manutenção.
- Compreensão da distinção entre testes de carga completa (com fixture populada) e testes de resiliência/fallback (com variáveis não setadas e sem arquivo).

**Demonstrated mastery of:**
- Organização idiomática de suítes de teste em Go com pastas `testdata/`.
- Centralização e encapsulamento de configurações padrão em nível de pacote (package-private).
- Criação e uso de `Makefile` para abstração de comandos complexos de compilação, testes e git.

**Left open:**
- Validação estrita de limites de configuração (ex: portas HTTP restritas entre 1-65535 e URLs de webhook formatadas com regex/parser estrito).
