import { useEffect, useState } from "react";
import { Plus, Trash2, X, AlertCircle, Loader2 } from "lucide-react";
import {
  createTarget,
  deleteTarget,
  extractApiErrorMessage,
  updateTarget,
} from "@/lib/api";
import {
  HTTP_METHODS,
  formValuesToPayload,
  methodSupportsBody,
  targetToFormValues,
  validateTargetForm,
  type TargetFormValues,
} from "@/lib/targetForm";
import type { HttpMethod, Target } from "@/types/check";

interface TargetModalProps {
  isOpen: boolean;
  target: Target | null;
  onClose: () => void;
  onSaved: (target: Target, mode: "create" | "update") => void;
  onDeleted: (targetId: string) => void;
}

export function TargetModal({
  isOpen,
  target,
  onClose,
  onSaved,
  onDeleted,
}: TargetModalProps) {
  const [values, setValues] = useState<TargetFormValues>(() =>
    targetToFormValues(target),
  );
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [apiError, setApiError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);

  useEffect(() => {
    if (!isOpen) {
      return;
    }
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !submitting) {
        onClose();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isOpen, submitting, onClose]);

  if (!isOpen) {
    return null;
  }

  const isEditing = target !== null;
  const showBodyEditor = methodSupportsBody(values.method);

  const updateField = <K extends keyof TargetFormValues>(
    field: K,
    value: TargetFormValues[K],
  ) => {
    setValues((prev) => ({ ...prev, [field]: value }));
    if (fieldErrors[field]) {
      setFieldErrors((prev) => {
        const next = { ...prev };
        delete next[field];
        return next;
      });
    }
  };

  const handleAddHeader = () => {
    setValues((prev) => ({
      ...prev,
      headers: [...prev.headers, { key: "", value: "" }],
    }));
  };

  const handleHeaderChange = (
    index: number,
    part: "key" | "value",
    text: string,
  ) => {
    setValues((prev) => ({
      ...prev,
      headers: prev.headers.map((entry, idx) =>
        idx === index ? { ...entry, [part]: text } : entry,
      ),
    }));
    if (fieldErrors.headers) {
      setFieldErrors((prev) => {
        const next = { ...prev };
        delete next.headers;
        return next;
      });
    }
  };

  const handleRemoveHeader = (index: number) => {
    setValues((prev) => ({
      ...prev,
      headers: prev.headers.filter((_, idx) => idx !== index),
    }));
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setApiError(null);

    const validationErrors = validateTargetForm(values);
    if (Object.keys(validationErrors).length > 0) {
      setFieldErrors(validationErrors);
      return;
    }

    const payload = formValuesToPayload(values);
    setSubmitting(true);

    try {
      if (isEditing && target) {
        const updated = await updateTarget(target.id, payload);
        onSaved(updated, "update");
      } else {
        const created = await createTarget(payload);
        onSaved(created, "create");
      }
      onClose();
    } catch (error) {
      setApiError(
        extractApiErrorMessage(
          error,
          "Não foi possível salvar o alvo. Verifique os dados e tente novamente.",
        ),
      );
    } finally {
      setSubmitting(false);
    }
  };

  const handleDelete = async () => {
    if (!target) {
      return;
    }
    setApiError(null);
    setSubmitting(true);
    try {
      await deleteTarget(target.id);
      onDeleted(target.id);
      onClose();
    } catch (error) {
      setApiError(
        extractApiErrorMessage(error, "Não foi possível excluir o alvo."),
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 backdrop-blur-sm p-4"
      onClick={() => {
        if (!submitting) {
          onClose();
        }
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="target-modal-title"
        className="w-full max-w-2xl max-h-[90vh] overflow-y-auto rounded-2xl bg-slate-900 border border-slate-800 shadow-2xl text-slate-100"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800 sticky top-0 bg-slate-900/95 backdrop-blur-sm z-10">
          <div>
            <h2 id="target-modal-title" className="text-lg font-bold text-white">
              {isEditing ? "Editar Alvo Monitorado" : "Novo Alvo Monitorado"}
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Configure endpoint, método HTTP, timeout e critérios de validação.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            aria-label="Fechar modal"
            className="p-2 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors cursor-pointer disabled:opacity-50"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-6 space-y-5">
          {apiError && (
            <div className="flex items-start gap-2.5 p-3.5 rounded-xl bg-rose-950/60 border border-rose-800 text-rose-200 text-sm">
              <AlertCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
              <span>{apiError}</span>
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label
                htmlFor="target-name"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Nome do Serviço *
              </label>
              <input
                id="target-name"
                name="name"
                type="text"
                value={values.name}
                onChange={(e) => updateField("name", e.target.value)}
                placeholder="Ex: API de Autenticação"
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
              />
              {fieldErrors.name && (
                <p className="text-xs text-rose-400 mt-1">{fieldErrors.name}</p>
              )}
            </div>

            <div>
              <label
                htmlFor="target-method"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Método HTTP
              </label>
              <select
                id="target-method"
                name="method"
                value={values.method}
                onChange={(e) =>
                  updateField("method", e.target.value as HttpMethod)
                }
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm text-white focus:outline-none focus:border-indigo-500"
              >
                {HTTP_METHODS.map((method) => (
                  <option key={method} value={method}>
                    {method}
                  </option>
                ))}
              </select>
              {fieldErrors.method && (
                <p className="text-xs text-rose-400 mt-1">
                  {fieldErrors.method}
                </p>
              )}
            </div>
          </div>

          <div>
            <label
              htmlFor="target-url"
              className="block text-xs font-medium text-slate-300 mb-1.5"
            >
              URL do Endpoint *
            </label>
            <input
              id="target-url"
              name="url"
              type="url"
              value={values.url}
              onChange={(e) => updateField("url", e.target.value)}
              placeholder="https://servico.exemplo.com/health"
              className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
            />
            {fieldErrors.url && (
              <p className="text-xs text-rose-400 mt-1">{fieldErrors.url}</p>
            )}
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label
                htmlFor="target-timeout"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Timeout (ms)
              </label>
              <input
                id="target-timeout"
                name="timeoutMs"
                type="number"
                min={0}
                step="any"
                value={values.timeoutMs}
                onChange={(e) => updateField("timeoutMs", e.target.value)}
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm font-mono text-white focus:outline-none focus:border-indigo-500"
              />
              {fieldErrors.timeoutMs && (
                <p className="text-xs text-rose-400 mt-1">
                  {fieldErrors.timeoutMs}
                </p>
              )}
            </div>

            <div>
              <label
                htmlFor="target-interval"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Intervalo (s)
              </label>
              <input
                id="target-interval"
                name="intervalSec"
                type="number"
                min={0}
                step="any"
                value={values.intervalSec}
                onChange={(e) => updateField("intervalSec", e.target.value)}
                placeholder="Padrão (MONITOR_INTERVAL)"
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
              />
              {fieldErrors.intervalSec ? (
                <p className="text-xs text-rose-400 mt-1">
                  {fieldErrors.intervalSec}
                </p>
              ) : (
                <p className="text-[11px] text-slate-400 mt-1">
                  Vazio ou 0 usa o padrão do servidor.
                </p>
              )}
            </div>

            <div>
              <label
                htmlFor="target-expected-status"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Status HTTP Esperados
              </label>
              <input
                id="target-expected-status"
                name="expectedStatusText"
                type="text"
                value={values.expectedStatusText}
                onChange={(e) =>
                  updateField("expectedStatusText", e.target.value)
                }
                placeholder="200, 201, 204"
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
              />
              {fieldErrors.expectedStatusText && (
                <p className="text-xs text-rose-400 mt-1">
                  {fieldErrors.expectedStatusText}
                </p>
              )}
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label
                htmlFor="target-tags"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Tags (separadas por vírgula)
              </label>
              <input
                id="target-tags"
                name="tagsText"
                type="text"
                value={values.tagsText}
                onChange={(e) => updateField("tagsText", e.target.value)}
                placeholder="prod, api, auth"
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
              />
            </div>

            <div>
              <label
                htmlFor="target-body-match"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Padrão Regex (Go RE2) no Body (Opcional)
              </label>
              <input
                id="target-body-match"
                name="expectedBodyMatch"
                type="text"
                value={values.expectedBodyMatch}
                onChange={(e) =>
                  updateField("expectedBodyMatch", e.target.value)
                }
                placeholder='Ex: "status"\s*:\s*"ok" ou \[OK\]'
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-sm font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
              />
              <p className="text-[11px] text-slate-400 mt-1">
                Avaliado como expressão regular (Go RE2). Para texto literal
                com metacaracteres como <code className="text-slate-300">[OK]</code>,
                escape com barra invertida (ex.:{" "}
                <code className="text-slate-300">\[OK\]</code>).
              </p>
            </div>
          </div>

          {/* Headers Key/Value Editor */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-slate-300">
                Cabeçalhos HTTP (Opcional)
              </span>
              <button
                type="button"
                onClick={handleAddHeader}
                className="inline-flex items-center gap-1 text-xs font-medium text-indigo-400 hover:text-indigo-300 cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5" />
                Adicionar Header
              </button>
            </div>

            {values.headers.length > 0 && (
              <div className="space-y-2">
                {values.headers.map((header, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <input
                      type="text"
                      value={header.key}
                      onChange={(e) =>
                        handleHeaderChange(index, "key", e.target.value)
                      }
                      placeholder="Nome (ex: Authorization)"
                      className="flex-1 px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-xs font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
                    />
                    <input
                      type="text"
                      value={header.value}
                      onChange={(e) =>
                        handleHeaderChange(index, "value", e.target.value)
                      }
                      placeholder="Valor"
                      className="flex-1 px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-xs font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
                    />
                    <button
                      type="button"
                      onClick={() => handleRemoveHeader(index)}
                      aria-label="Remover cabeçalho"
                      className="p-1.5 rounded-lg text-slate-400 hover:text-rose-400 hover:bg-slate-800 cursor-pointer"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                ))}
              </div>
            )}
            {fieldErrors.headers && (
              <p className="text-xs text-rose-400">{fieldErrors.headers}</p>
            )}
          </div>

          {showBodyEditor && (
            <div>
              <label
                htmlFor="target-body-json"
                className="block text-xs font-medium text-slate-300 mb-1.5"
              >
                Corpo JSON ({values.method}, Opcional)
              </label>
              <textarea
                id="target-body-json"
                name="bodyText"
                rows={4}
                value={values.bodyText}
                onChange={(e) => updateField("bodyText", e.target.value)}
                placeholder='{"ping": true}'
                className="w-full px-3.5 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
              />
              {fieldErrors.bodyText && (
                <p className="text-xs text-rose-400 mt-1">
                  {fieldErrors.bodyText}
                </p>
              )}
            </div>
          )}

          <label className="flex items-center gap-3 p-3 rounded-xl bg-slate-950/70 border border-slate-800 cursor-pointer select-none">
            <input
              type="checkbox"
              checked={values.enabled}
              onChange={(e) => updateField("enabled", e.target.checked)}
              className="w-4 h-4 accent-indigo-500 rounded cursor-pointer"
            />
            <div>
              <div className="text-sm font-medium text-slate-200">
                Monitoramento ativo
              </div>
              <div className="text-xs text-slate-400">
                Quando desativado, o serviço permanece salvo mas não recebe
                requisições de health check.
              </div>
            </div>
          </label>

          {/* Footer actions */}
          <div className="flex flex-col-reverse sm:flex-row sm:items-center justify-between gap-3 pt-4 border-t border-slate-800">
            <div>
              {isEditing && !confirmingDelete && (
                <button
                  type="button"
                  disabled={submitting}
                  onClick={() => setConfirmingDelete(true)}
                  className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium text-rose-400 border border-rose-900/60 bg-rose-950/30 hover:bg-rose-950/70 transition-colors cursor-pointer disabled:opacity-50"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  Excluir Alvo
                </button>
              )}

              {isEditing && confirmingDelete && (
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    disabled={submitting}
                    onClick={handleDelete}
                    className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold bg-rose-600 hover:bg-rose-500 text-white transition-colors cursor-pointer disabled:opacity-50"
                  >
                    Confirmar Exclusão
                  </button>
                  <button
                    type="button"
                    disabled={submitting}
                    onClick={() => setConfirmingDelete(false)}
                    className="px-3 py-2 rounded-lg text-xs font-medium text-slate-300 hover:text-white bg-slate-800 cursor-pointer disabled:opacity-50"
                  >
                    Cancelar
                  </button>
                </div>
              )}
            </div>

            <div className="flex items-center justify-end gap-2.5">
              <button
                type="button"
                disabled={submitting}
                onClick={onClose}
                className="px-4 py-2 rounded-lg text-xs font-medium bg-slate-800 text-slate-300 hover:text-white hover:bg-slate-700 transition-colors cursor-pointer disabled:opacity-50"
              >
                Cancelar
              </button>
              <button
                type="submit"
                disabled={submitting}
                className="inline-flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-semibold bg-indigo-600 hover:bg-indigo-500 text-white transition-colors cursor-pointer disabled:opacity-50"
              >
                {submitting && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
                {isEditing ? "Salvar Alterações" : "Criar Alvo"}
              </button>
            </div>
          </div>
        </form>
      </div>
    </div>
  );
}
