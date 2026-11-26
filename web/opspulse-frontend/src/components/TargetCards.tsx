import { useMemo, useState } from "react";
import {
  Activity,
  ExternalLink,
  History,
  Pencil,
  Plus,
  Power,
  RefreshCw,
  Search,
  ShieldCheck,
  Trash2,
} from "lucide-react";
import { formatLatencyMs } from "@/lib/insights";
import { formatIntervalLabel, nsToMs } from "@/lib/targetForm";
import {
  buildLiveResultLookup,
  classifyTargetAvailability,
  findLiveResultForTarget,
  findOrphanLiveResults,
  loadAvailabilityFilter,
  saveAvailabilityFilter,
  type AvailabilityFilter,
} from "@/lib/targetStatus";
import type { CheckResult, Target } from "@/types/check";

const AVAILABILITY_FILTER_OPTIONS: readonly {
  value: AvailabilityFilter;
  label: string;
}[] = [
  { value: "all", label: "Todos" },
  { value: "up", label: "UP" },
  { value: "down", label: "DOWN" },
  { value: "paused", label: "Pausados" },
  { value: "pending", label: "Aguardando" },
];

function getBrowserStorage() {
  return typeof window !== "undefined" ? window.localStorage : null;
}

interface TargetCardsProps {
  targets: Target[];
  liveResults: CheckResult[];
  loading: boolean;
  error: string | null;
  busyTargetId: string | null;
  onCreate: () => void;
  onEdit: (target: Target) => void;
  onToggleEnabled: (target: Target) => void;
  onDelete: (target: Target) => void;
  onViewCheckLogs: (targetId: string) => void;
  onRefresh: () => void;
}

export function TargetCards({
  targets,
  liveResults,
  loading,
  error,
  busyTargetId,
  onCreate,
  onEdit,
  onToggleEnabled,
  onDelete,
  onViewCheckLogs,
  onRefresh,
}: TargetCardsProps) {
  const [searchQuery, setSearchQuery] = useState("");
  const [availabilityFilter, setAvailabilityFilter] =
    useState<AvailabilityFilter>(() =>
      loadAvailabilityFilter(getBrowserStorage()),
    );
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

  const handleAvailabilityFilterChange = (next: AvailabilityFilter) => {
    setAvailabilityFilter(next);
    saveAvailabilityFilter(getBrowserStorage(), next);
  };

  const liveByUrl = useMemo(
    () => buildLiveResultLookup(liveResults),
    [liveResults],
  );

  const availabilityCounts = useMemo(() => {
    const counts: Record<AvailabilityFilter, number> = {
      all: targets.length,
      up: 0,
      down: 0,
      paused: 0,
      pending: 0,
    };
    for (const target of targets) {
      const live = findLiveResultForTarget(target, liveByUrl);
      const state = classifyTargetAvailability(target, live);
      counts[state] += 1;
    }
    return counts;
  }, [targets, liveByUrl]);

  const filteredTargets = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    return targets.filter((target) => {
      if (availabilityFilter !== "all") {
        const live = findLiveResultForTarget(target, liveByUrl);
        if (classifyTargetAvailability(target, live) !== availabilityFilter) {
          return false;
        }
      }
      if (!q) {
        return true;
      }
      const inName = target.name.toLowerCase().includes(q);
      const inUrl = target.url.toLowerCase().includes(q);
      const inTags = (target.tags ?? []).some((tag) =>
        tag.toLowerCase().includes(q),
      );
      return inName || inUrl || inTags;
    });
  }, [targets, searchQuery, availabilityFilter, liveByUrl]);

  const orphanLiveResults = useMemo(() => {
    const orphans = findOrphanLiveResults(targets, liveResults);
    const q = searchQuery.trim().toLowerCase();
    return orphans.filter((live) => {
      if (availabilityFilter === "paused" || availabilityFilter === "pending") {
        return false;
      }
      if (availabilityFilter === "up" && !live.IsUp) {
        return false;
      }
      if (availabilityFilter === "down" && live.IsUp) {
        return false;
      }
      if (!q) {
        return true;
      }
      return (
        live.Name.toLowerCase().includes(q) ||
        live.URL.toLowerCase().includes(q)
      );
    });
  }, [targets, liveResults, searchQuery, availabilityFilter]);

  return (
    <section className="space-y-4">
      {/* Section Header & Actions */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-slate-900/80 border border-slate-800 rounded-xl p-4">
        <div>
          <h2 className="text-base font-semibold text-white flex items-center gap-2">
            <Activity className="w-4 h-4 text-indigo-400" />
            Serviços Monitorados
          </h2>
          <p className="text-xs text-slate-400 mt-0.5">
            Configuração persistida combinada com telemetria SSE em tempo real.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2.5">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Filtrar nome, URL ou tag..."
              className="pl-8 pr-3 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-xs text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500 w-52"
            />
          </div>

          <button
            type="button"
            onClick={onRefresh}
            disabled={loading}
            className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-slate-950 border border-slate-700 text-slate-300 hover:text-white hover:border-slate-600 transition-colors cursor-pointer disabled:opacity-50"
          >
            <RefreshCw
              className={`w-3.5 h-3.5 ${loading ? "animate-spin" : ""}`}
            />
            Atualizar
          </button>

          <button
            type="button"
            onClick={onCreate}
            className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg text-xs font-semibold bg-indigo-600 hover:bg-indigo-500 text-white transition-colors cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            Novo Alvo
          </button>
        </div>
      </div>

      {/* Availability Filter Bar (persisted in localStorage) */}
      <div
        role="group"
        aria-label="Filtrar por disponibilidade"
        className="flex flex-wrap items-center gap-2"
      >
        {AVAILABILITY_FILTER_OPTIONS.map((option) => {
          const active = availabilityFilter === option.value;
          const count = availabilityCounts[option.value];
          return (
            <button
              key={option.value}
              type="button"
              aria-pressed={active}
              onClick={() => handleAvailabilityFilterChange(option.value)}
              className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors cursor-pointer ${
                active
                  ? "bg-indigo-600/20 border-indigo-500 text-indigo-200"
                  : "bg-slate-900/80 border-slate-800 text-slate-400 hover:text-white hover:border-slate-700"
              }`}
            >
              <span>{option.label}</span>
              <span
                className={`px-1.5 py-0.5 rounded-full text-[10px] font-mono ${
                  active
                    ? "bg-indigo-500/30 text-indigo-100"
                    : "bg-slate-800 text-slate-400"
                }`}
              >
                {count}
              </span>
            </button>
          );
        })}
      </div>

      {error && (
        <div className="p-3.5 rounded-xl bg-rose-950/60 border border-rose-800 text-rose-200 text-xs">
          {error}
        </div>
      )}

      {loading && targets.length === 0 && orphanLiveResults.length === 0 ? (
        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-10 text-center text-sm text-slate-400">
          Carregando alvos monitorados...
        </div>
      ) : filteredTargets.length === 0 && orphanLiveResults.length === 0 ? (
        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-10 text-center space-y-3">
          <p className="text-sm text-slate-400">
            {targets.length === 0
              ? "Nenhum alvo cadastrado ainda. Crie o primeiro serviço para iniciar o monitoramento."
              : "Nenhum alvo corresponde ao filtro informado."}
          </p>
          {targets.length === 0 && (
            <button
              type="button"
              onClick={onCreate}
              className="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-semibold bg-indigo-600 hover:bg-indigo-500 text-white transition-colors cursor-pointer"
            >
              <Plus className="w-4 h-4" />
              Cadastrar Primeiro Alvo
            </button>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredTargets.map((target) => {
            const live = findLiveResultForTarget(target, liveByUrl);
            const isBusy = busyTargetId === target.id;
            const timeoutMs = nsToMs(target.timeout);
            const expectedCodes = target.expected_status ?? [];
            const tags = target.tags ?? [];
            const isConfirmingDelete = confirmDeleteId === target.id;

            let badgeClass =
              "bg-slate-800 text-slate-300 border-slate-700";
            let badgeLabel = "Aguardando";
            let accentBorder = "border-slate-800 hover:border-slate-700";

            if (!target.enabled) {
              badgeClass =
                "bg-amber-950/60 text-amber-300 border-amber-800/70";
              badgeLabel = "Pausado";
              accentBorder = "border-slate-800/80 opacity-85";
            } else if (live) {
              if (live.IsUp) {
                badgeClass =
                  "bg-emerald-950/60 text-emerald-400 border-emerald-800/70";
                badgeLabel = "UP";
                accentBorder = "border-slate-800 hover:border-emerald-800/60";
              } else {
                badgeClass =
                  "bg-rose-950/60 text-rose-400 border-rose-800/70";
                badgeLabel = "DOWN";
                accentBorder = "border-rose-900/60 hover:border-rose-700/70";
              }
            }

            return (
              <article
                key={target.id}
                className={`bg-slate-900/85 border ${accentBorder} rounded-xl p-5 flex flex-col justify-between gap-4 shadow-lg transition-colors`}
              >
                <div className="space-y-3">
                  {/* Top row: Method + Status */}
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="px-2 py-0.5 rounded text-[11px] font-mono font-semibold bg-indigo-950/80 text-indigo-300 border border-indigo-800/60">
                          {target.method || "GET"}
                        </span>
                        <h3 className="text-sm font-semibold text-white truncate">
                          {target.name}
                        </h3>
                      </div>
                      <a
                        href={target.url}
                        target="_blank"
                        rel="noreferrer"
                        className="mt-1 inline-flex items-center gap-1 text-xs font-mono text-slate-400 hover:text-indigo-300 break-all"
                      >
                        <span>{target.url}</span>
                        <ExternalLink className="w-3 h-3 shrink-0" />
                      </a>
                    </div>

                    <span
                      className={`shrink-0 inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-semibold border ${badgeClass}`}
                    >
                      {target.enabled && live && (
                        <span
                          className={`w-2 h-2 rounded-full ${
                            live.IsUp ? "bg-emerald-400" : "bg-rose-400"
                          }`}
                        />
                      )}
                      {badgeLabel}
                    </span>
                  </div>

                  {/* Metrics Grid */}
                  <div className="grid grid-cols-2 gap-2.5 bg-slate-950/70 border border-slate-800/80 rounded-lg p-3 text-xs">
                    <div>
                      <div className="text-slate-500 text-[11px]">
                        Status HTTP
                      </div>
                      <div className="font-mono text-slate-200 mt-0.5">
                        {live
                          ? live.StatusCode > 0
                            ? live.StatusCode
                            : "Erro de rede"
                          : "--"}
                        <span className="text-slate-500 ml-1">
                          (esp. {expectedCodes.join(",") || "200"})
                        </span>
                      </div>
                    </div>

                    <div>
                      <div className="text-slate-500 text-[11px]">
                        Latência / Timeout / Intervalo
                      </div>
                      <div className="font-mono text-amber-300/90 mt-0.5">
                        {live ? formatLatencyMs(live.Latency) : "--"}
                        <span className="text-slate-500 ml-1">
                          / {timeoutMs} ms · {formatIntervalLabel(target.interval)}
                        </span>
                      </div>
                    </div>
                  </div>

                  {/* Tags & Body Match */}
                  {(tags.length > 0 || target.expected_body_match) && (
                    <div className="flex flex-wrap items-center gap-1.5">
                      {tags.map((tag) => (
                        <span
                          key={tag}
                          className="px-2 py-0.5 rounded-md text-[11px] bg-slate-800/80 text-slate-300 border border-slate-700/70"
                        >
                          #{tag}
                        </span>
                      ))}
                      {target.expected_body_match && (
                        <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md text-[11px] bg-slate-800/80 text-cyan-300 border border-cyan-900/60 font-mono">
                          <ShieldCheck className="w-3 h-3" />
                          match: {target.expected_body_match}
                        </span>
                      )}
                    </div>
                  )}

                  {/* Diagnostic */}
                  <div className="text-xs rounded-lg px-3 py-2 bg-slate-950/50 border border-slate-800/70">
                    {!target.enabled ? (
                      <span className="text-amber-300/90">
                        Monitoramento pausado para este alvo.
                      </span>
                    ) : live?.Error ? (
                      <span className="text-rose-400 font-mono break-all">
                        {live.Error}
                      </span>
                    ) : live ? (
                      <span className="text-emerald-400/90">
                        Operacional — última checagem aprovada.
                      </span>
                    ) : (
                      <span className="text-slate-500">
                        Aguardando próximo ciclo de verificação...
                      </span>
                    )}
                  </div>
                </div>

                {/* Card Actions */}
                <div className="pt-3 border-t border-slate-800/80 flex flex-wrap items-center justify-between gap-2">
                  <div className="flex items-center gap-1.5">
                    <button
                      type="button"
                      disabled={isBusy}
                      onClick={() => onToggleEnabled(target)}
                      title={target.enabled ? "Pausar alvo" : "Ativar alvo"}
                      className={`inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-medium border transition-colors cursor-pointer disabled:opacity-50 ${
                        target.enabled
                          ? "bg-slate-950 border-slate-700 text-slate-300 hover:text-amber-300 hover:border-amber-800"
                          : "bg-emerald-950/50 border-emerald-800 text-emerald-300 hover:bg-emerald-950"
                      }`}
                    >
                      <Power className="w-3.5 h-3.5" />
                      {target.enabled ? "Pausar" : "Ativar"}
                    </button>

                    <button
                      type="button"
                      onClick={() => onViewCheckLogs(target.id)}
                      className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-medium bg-slate-950 border border-slate-700 text-slate-300 hover:text-indigo-300 hover:border-indigo-800 transition-colors cursor-pointer"
                    >
                      <History className="w-3.5 h-3.5" />
                      Checklogs
                    </button>
                  </div>

                  <div className="flex items-center gap-1.5">
                    <button
                      type="button"
                      disabled={isBusy}
                      onClick={() => onEdit(target)}
                      aria-label={`Editar ${target.name}`}
                      className="inline-flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-medium bg-slate-950 border border-slate-700 text-slate-300 hover:text-white hover:border-slate-500 transition-colors cursor-pointer disabled:opacity-50"
                    >
                      <Pencil className="w-3.5 h-3.5" />
                      Editar
                    </button>

                    {isConfirmingDelete ? (
                      <div className="inline-flex items-center gap-1">
                        <button
                          type="button"
                          disabled={isBusy}
                          onClick={() => {
                            setConfirmDeleteId(null);
                            onDelete(target);
                          }}
                          className="px-2 py-1.5 rounded-lg text-xs font-semibold bg-rose-600 hover:bg-rose-500 text-white cursor-pointer disabled:opacity-50"
                        >
                          Confirmar
                        </button>
                        <button
                          type="button"
                          onClick={() => setConfirmDeleteId(null)}
                          className="px-2 py-1.5 rounded-lg text-xs bg-slate-800 text-slate-300 hover:text-white cursor-pointer"
                        >
                          Não
                        </button>
                      </div>
                    ) : (
                      <button
                        type="button"
                        disabled={isBusy}
                        onClick={() => setConfirmDeleteId(target.id)}
                        aria-label={`Excluir ${target.name}`}
                        className="p-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-400 hover:text-rose-400 hover:border-rose-900 transition-colors cursor-pointer disabled:opacity-50"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    )}
                  </div>
                </div>
              </article>
            );
          })}

          {orphanLiveResults.map((live, index) => (
            <article
              key={`live-${live.URL}-${index}`}
              className="bg-slate-900/60 border border-slate-800 rounded-xl p-5 flex flex-col justify-between gap-3"
            >
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <h3 className="text-sm font-semibold text-white truncate">
                    {live.Name || live.URL}
                  </h3>
                  <div className="text-xs font-mono text-slate-400 break-all mt-1">
                    {live.URL}
                  </div>
                </div>
                <span
                  className={`shrink-0 px-2.5 py-1 rounded-full text-xs font-semibold border ${
                    live.IsUp
                      ? "bg-emerald-950/60 text-emerald-400 border-emerald-800/70"
                      : "bg-rose-950/60 text-rose-400 border-rose-800/70"
                  }`}
                >
                  {live.IsUp ? "UP" : "DOWN"}
                </span>
              </div>

              <div className="grid grid-cols-2 gap-2 bg-slate-950/70 border border-slate-800 rounded-lg p-3 text-xs">
                <div>
                  <div className="text-slate-500 text-[11px]">Status HTTP</div>
                  <div className="font-mono text-slate-200 mt-0.5">
                    {live.StatusCode > 0 ? live.StatusCode : "Erro de rede"}
                  </div>
                </div>
                <div>
                  <div className="text-slate-500 text-[11px]">Latência</div>
                  <div className="font-mono text-amber-300/90 mt-0.5">
                    {formatLatencyMs(live.Latency)}
                  </div>
                </div>
              </div>

              <div className="text-xs text-slate-400 font-mono break-all">
                {live.Error || "Operacional (telemetria SSE)"}
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
