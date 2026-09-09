┌────────────────────────────────────────────────────────────────────────┐
│ PASSO 1: Implementar o JSONTargetLoader (Lendo de targets.json)         │
├────────────────────────────────────────────────────────────────────────┤
│ PASSO 2: Criar o arquivo de exemplo targets.json na raiz do projeto    │
├────────────────────────────────────────────────────────────────────────┤
│ PASSO 3: Criar um Loader com Fallback Inteligente                      │
│         (Tenta targets.json; se não existir, usa TARGET_URLS do .env)  │
├────────────────────────────────────────────────────────────────────────┤
│ PASSO 4: Atualizar os Testes Unitários (config_test e checker_test)    │
├────────────────────────────────────────────────────────────────────────┤
│ PASSO 5: Conectar tudo no main.go e rodar os testes gerais             │
└────────────────────────────────────────────────────────────────────────┘
