---
date: 2026-09-30
project: go-project
topic: react-hook-sse-lifecycle
---

**Where they got stuck:**
- Uso de variável local `let eventSourceRef` dentro do corpo da função do hook customizado `useSSE`, o que causava a perda da referência à instância a cada re-renderização do React.
- Chamada direta de `eventSource.close()` ao pausar o monitoramento que impedia a reconexão automática ou atualização posterior do estado da UI.

**What unlocked it:**
- Migração para `useRef<EventSource | null>(null)` persistindo a referência viva entre renderizações sem disparar re-render ciclos.
- Encapsulamento correto do ciclo de vida no hook: limpeza adequada no cleanup do `useEffect` e controle explícito das ações de `stop()` e `restart()`.

**Demonstrated mastery of:**
- Arquitetura de componentes e hooks customizados no React 19 com TypeScript.
- Gerenciamento de efeitos colaterais (`useEffect`), referências mutáveis (`useRef`) e estados derivados.
- Consumo de eventos em tempo real no browser via API nativa `EventSource` com tratamento de eventos de erro, conexão e dados.
- Estruturação modular no frontend (`types/`, `hooks/`, `components/`, `pages/`) integrada ao Tailwind CSS v4.

**Left open:**
- Estratégia de reconexão automática com backoff exponencial no frontend caso o servidor caia.
