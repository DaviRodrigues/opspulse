---
date: 2026-10-01
project: go-project
topic: strategy-pattern-file-loaders
---

**Where they got stuck:**
- Centralização de múltiplos formatos (JSON, YAML) dentro de uma única struct com condicionais rígidas, dificultando testes unitários e acoplando a leitura de I/O em disco com a lógica de unmarshaling.
- Transição de dependência de biblioteca YAML arquivada (`gopkg.in/yaml.v2` para `gopkg.in/yaml.v3`) e manutenção da flexibilidade de novos parsers sem quebrar os testes existentes.

**What unlocked it:**
- Adoção do padrão Strategy / Higher-Order Functions: separação do carregador (`FileDefault`) da função de decodificação (`UnmarshalFunc`), permitindo injetar `json.Unmarshal` ou `yaml.Unmarshal` dinamicamente.
- Criação de uma interface clara `TargetLoader` e isolamento dos testes com dados em memória (`[]byte`) sem depender de arquivos físicos no disco.

**Demonstrated mastery of:**
- Aplicação prática de Go interfaces para desacoplamento e extensão (Open/Closed Principle).
- Uso de funções de primeira classe (First-Class Functions) como estratégias de unmarshal.
- Padrão Factory (`NewFile`, `NewFileReader`) para orquestração de instâncias a partir de extensões de arquivo (`.json`, `.yaml`, `.yml`).

**Left open:**
- Suporte a carregamento de targets a partir de fontes remotas (URLs, S3, etcd ou bancos de dados).
