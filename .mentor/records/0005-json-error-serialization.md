---
date: 2026-09-29
project: go-project
topic: json-error-serialization
---

**Where they got stuck:**
- Campo de tipo `error` nativo de Go dentro da struct `CheckResult` serializava como `{}` no payload JSON enviado aos clientes SSE, ocultando a mensagem real de falha de conexão/timeout.

**What unlocked it:**
- Compreensão do funcionamento do pacote `encoding/json` e da reflexão em Go: tipos `error` são interfaces cujas implementações padrão (como `errors.errorString`) possuem campos não exportados (minúsculos), resultando em objetos vazios na serialização padrão.
- Simplificação do campo `CheckResult.Error` para `string` com tag `json:"Error,omitempty"` e tratamento seguro convertendo `err.Error()` ou `http.StatusText(code)` no momento da checagem.

**Demonstrated mastery of:**
- Regras de exportação de campos e serialização JSON idiomática em Go.
- Modelagem de contratos de dados (DTOs) para APIs públicas e eventos em tempo real.
- Diferenciação entre tipos de controle de fluxo de erros internos e representação de erros para consumo externo.

**Left open:**
- Enriquecimento do modelo de erro com códigos de classificação (ex: `DNS_FAILURE`, `HTTP_TIMEOUT`, `SSL_ERROR`).
