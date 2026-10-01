#!/bin/bash
set -e # Interrompe imediatamente se algum comando falhar

CURRENT_BRANCH=$(git branch --show-current)

if [[ -n $(git status --porcelain) ]]; then
  echo "❌ Erro: Você possui alterações não commitadas. Faça commit ou stash antes de sincronizar."
  exit 1
fi

echo "🔄 Atualizando a branch dev..."
git checkout dev
git pull origin dev || true

declare -a my_branches=(
  "feat/add_api_module_setup"
  "feat/add_manager_db_storage"
  "feat/multi_notifiers_channels"
  "feat/terraform_ci_cd_deploy"
)

for b in "${my_branches[@]}"; do
  echo "🔀 Mesclando 'dev' em '$b'..."
  git checkout "$b"
  if ! git merge dev; then
    echo "⚠️ Conflito de merge detectado na branch $b! Resolva o conflito manualmente."
    exit 1
  fi
  git checkout dev
done

git checkout "$CURRENT_BRANCH"
echo "✅ Todas as branches foram sincronizadas com sucesso!"