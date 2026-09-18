# Pacote 3: Persistência NoSQL, Histórico de Incidentes & Métricas (MongoDB)

> Guia de arquitetura, modelagem NoSQL e integração do MongoDB ao OpsPulse em Go.

---

## 1. Funcionamento do MongoDB

O **MongoDB** é um banco de dados NoSQL orientado a **Documentos**.

* Em vez de **Tabelas, Linhas e Colunas** (como PostgreSQL ou MySQL), o MongoDB organiza os dados em **Bancos $\to$ Coleções (Collections) $\to$ Documentos (BSON/JSON)**.
* Cada documento possui um identificador único padrão gerado automaticamente chamado `_id` do tipo `primitive.ObjectID` (12 bytes contendo timestamp + machine id + process id + counter).
* Em Go, os documentos são mapeados diretamente para `structs` usando tags `bson:"nome_do_campo"`.

---

## 2. Modelagem & Relacionamento entre Coleções

No NoSQL não existe o conceito de `JOIN` relacional tradicional rígido. Você escolhe entre duas estratégias:

### Estratégia A: Documentos Embutidos (*Embedding / Desnormalização*)

* Guarda dados filhos diretamente dentro de um documento pai (como structs aninhadas ou slices).
* **Quando usar:** Relações $1:1$ ou $1:\text{poucos}$ onde os dados são sempre consultados juntos.
* **Exemplo:** Guardar os headers customizados ou detalhes do erro dentro do log de checagem.

### Estratégia B: Referência (*Referencing / Normalização*)

* Guarda apenas o `_id` do documento pai na coleção filha (`target_id: ObjectID`).
* **Quando usar:** Relações $1:N$ com crescimento contínuo (ex: milhões de logs de checagem por URL).
* No MongoDB, se precisar cruzar dados referenciados, usamos o operador **`$lookup`** em um *Aggregation Pipeline*.

---

## 3. Estrutura do Banco de Dados

* Primeiramente será necessário construir o diagrama do banco de dados;
* Criar um diretório dentro do internal chamado models (onde vão ficar as structs do banco);
* Criar um diretório dentro do internal chamado storage para a conexão com o mongodb;
* Em seguida baixar as dependências para utilizá-lo posteriormente;

### Driver / ORM Utilizar em Go:

A comunidade utiliza o **Driver Oficial da MongoDB**:

* **Pacote oficial:** `go.mongodb.org/mongo-driver/v2/mongo`
* **Por que?**
  * É nativo, mantido pela equipe do MongoDB.
  * Altíssimo desempenho com pools de conexão e decodificação zero-alloc.
  * Mapeamento direto de Go Structs $\leftrightarrow$ BSON.

### Ferramentas:

* **Estratégia mais comum em Go:** Criação automática de índices na inicialização do repositório (`repository.EnsureIndexes(ctx)`).
* **Para versionamento estrito:** Ferramenta **`golang-migrate/migrate`** com suporte ao driver `mongodb`.

### Migrations no MongoDB:

No NoSQL não existe `ALTER TABLE` porque não há esquema rígido. Mas migrations ainda são fundamentais para:

1. **Garantir Índices (Index Migration):**
   * Índices únicos em URLs (`targets`).
   * Índices em `checked_at` para ordenação rápida.
2. **Atualização de Estrutura de Documentos (Data Patching):**
   * Se um campo mudar de formato, scripts de migração atualizam documentos em lote (`UpdateMany`).

### 1. `targets` (Configuração dos Serviços)

Guarda as URLs e parâmetros de monitoramento individuais.

---

Campos:

* ID
* Name
* URL
* Method (Pode ser enum, ou string em maiúsculo) // GET, POST, HEAD
* ExpectedStatus (pode ser nulo)
* Timeout 
* Headers (pode ser nulo)
* Body (pode ser nulo)
* Tags (pode ser nulo)
* ExpectedBodyMatch (pode ser nulo)
* CreatedAt
* UpdatedAt

### 2. `check_logs` (Histórico de Pings / Time-Series)

Guarda cada execução de teste de saúde.

---

> **Dica (Índice TTL):** Criamos um índice *Time-To-Live* para o MongoDB deletar logs com mais de 30 dias automaticamente sem encher o disco.

Campos:

* ID
* TargetID
* URL
* IsUp
* StatusCode
* LatencyMs
* Error
* CheckedAt

### 3. `incidents` (Gestão de Quedas & Downtime)

Registrado quando um serviço cai e atualizado quando ele volta, calculando o tempo total de indisponibilidade e SLA.

---

Campos:

* ID
* TargetID
* URL
* Status // "OPEN" ou "RESOLVED"
* StartedAt
* ResolvedAt
* DurationSeconds
* RootCause

---
