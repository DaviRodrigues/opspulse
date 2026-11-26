import { useEffect, useMemo, useState } from "react";
import {
  AlertTriangle,
  BarChart3,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock,
  Filter,
  Gauge,
  RefreshCw,
  RotateCcw,
  Search,
  XCircle,
  Zap,
} from "lucide-react";
import { extractApiErrorMessage, listCheckLogs, listTargets } from "@/lib/api";
import {
  computeCheckLogInsights,
  computeCheckLogsPagination,
  formatLatencyMs,
} from "@/lib/insights";
import type {
  CheckLog,
  CheckLogFilters,
  PaginationMeta,
  Target,
} from "@/types/check";

interface CheckLogsPageProps {
  initialTargetId?: string;
}

type TimePreset = "all" | "1h" | "24h" | "7d" | "30d" | "custom";

function presetToFromIso(preset: TimePreset): string | undefined {
  const now = Date.now();
  switch (preset) {
    case "1h":
      return new Date(now - 60 * 60 * 1000).toISOString();
    case "24h":
      return new Date(now - 24 * 60 * 60 * 1000).toISOString();
    case "7d":
      return new Date(now - 7 * 24 * 60 * 60 * 1000).toISOString();
    case "30d":
      return new Date(now - 30 * 24 * 60 * 60 * 1000).toISOString();
    default:
      return undefined;
  }
}

function formatDateTime(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return iso;
  }
  return date.toLocaleString("pt-BR");
}

export function CheckLogsPage({ initialTargetId = "" }: CheckLogsPageProps) {
  const [targets, setTargets] = useState<Target[]>([]);
  const [logs, setLogs] = useState<CheckLog[]>([]);
  const [pagination, setPagination] = useState<PaginationMeta>({
    total: 0,
    limit: 50,
    offset: 0,
  });
  const [settledRequestKey, setSettledRequestKey] = useState<string | null>(
    null,
  );
  const [error, setError] = useState<string | null>(null);

  const [targetId, setTargetId] = useState(initialTargetId);
  const [isUpFilter, setIsUpFilter] = useState<"all" | "true" | "false">("all");
  const [statusCodeInput, setStatusCodeInput] = useState("");
  const [timePreset, setTimePreset] = useState<TimePreset>("24h");
  const [fromLocal, setFromLocal] = useState("");
  const [toLocal, setToLocal] = useState("");
  const [search, setSearch] = useState("");
  const [limit, setLimit] = useState(25);
  const [offset, setOffset] = useState(0);
  const [refreshTick, setRefreshTick] = useState(0);

  useEffect(() => {
    let active = true;
    listTargets({ limit: 200, offset: 0 })
      .then((res) => {
        if (active) {
          setTargets(res.data);
        }
      })
      .catch(() => {
        // Dropdown permanece funcional mesmo se lista de targets falhar
      });
    return () => {
      active = false;
    };
  }, []);

  const queryState = useMemo((): {
    filters: CheckLogFilters | null;
    validationError: string | null;
  } => {
    const filters: CheckLogFilters = {
      limit,
      offset,
    };

    if (targetId) {
      filters.target_id = targetId;
    }
    if (isUpFilter === "true") {
      filters.is_up = true;
    } else if (isUpFilter === "false") {
      filters.is_up = false;
    }

    const trimmedCode = statusCodeInput.trim();
    if (trimmedCode) {
      if (!/^\d+$/.test(trimmedCode)) {
        return {
          filters: null,
          validationError:
            "Código HTTP inválido. Informe um número entre 0 e 599.",
        };
      }
      const code = Number.parseInt(trimmedCode, 10);
      if (code < 0 || code > 599) {
        return {
          filters: null,
          validationError: "Código HTTP deve estar entre 0 e 599.",
        };
      }
      filters.status_code = code;
    }

    if (search.trim()) {
      filters.search = search.trim();
    }

    if (timePreset === "custom") {
      if (fromLocal) {
        const fromDate = new Date(fromLocal);
        if (Number.isNaN(fromDate.getTime())) {
          return { filters: null, validationError: "Data inicial inválida." };
        }
        filters.from = fromDate.toISOString();
      }
      if (toLocal) {
        const toDate = new Date(toLocal);
        if (Number.isNaN(toDate.getTime())) {
          return { filters: null, validationError: "Data final inválida." };
        }
        filters.to = toDate.toISOString();
      }
      if (filters.from && filters.to && filters.from > filters.to) {
        return {
          filters: null,
          validationError:
            "A data inicial deve ser anterior ou igual à data final.",
        };
      }
    } else {
      const presetFrom = presetToFromIso(timePreset);
      if (presetFrom) {
        filters.from = presetFrom;
      }
    }

    return { filters, validationError: null };
  }, [
    limit,
    offset,
    targetId,
    isUpFilter,
    statusCodeInput,
    search,
    timePreset,
    fromLocal,
    toLocal,
  ]);

  const requestKey = useMemo(() => {
    if (!queryState.filters) {
      return null;
    }
    return JSON.stringify({ filters: queryState.filters, refreshTick });
  }, [queryState.filters, refreshTick]);

  useEffect(() => {
    if (!queryState.filters || !requestKey) {
      return;
    }

    const reqFilters = queryState.filters;
    const reqLimit = reqFilters.limit ?? 50;
    const reqOffset = reqFilters.offset ?? 0;

    let active = true;
    listCheckLogs(reqFilters)
      .then((result) => {
        if (!active) {
          return;
        }
        setLogs(result.data);
        setPagination(result.pagination);
        setError(null);
        const { clampedOffset } = computeCheckLogsPagination(
          result.pagination.total,
          reqLimit,
          reqOffset,
        );
        if (clampedOffset !== reqOffset) {
          setOffset(clampedOffset);
        }
      })
      .catch((err) => {
        if (!active) {
          return;
        }
        setError(
          extractApiErrorMessage(err, "Falha ao carregar histórico de checklogs."),
        );
      })
      .finally(() => {
        if (active) {
          setSettledRequestKey(requestKey);
        }
      });

    return () => {
      active = false;
    };
  }, [queryState.filters, requestKey]);

  const handleRefresh = () => {
    if (!queryState.filters) {
      return;
    }
    setError(null);
    setRefreshTick((prev) => prev + 1);
  };

  const displayError = queryState.validationError ?? error;
  const isFetching =
    requestKey !== null &&
    settledRequestKey !== requestKey &&
    !queryState.validationError;

  const insights = useMemo(() => computeCheckLogInsights(logs), [logs]);

  const handleResetFilters = () => {
    setTargetId("");
    setIsUpFilter("all");
    setStatusCodeInput("");
    setTimePreset("24h");
    setFromLocal("");
    setToLocal("");
    setSearch("");
    setLimit(50);
    setOffset(0);
  };

  const { totalPages, currentPage, clampedOffset } = computeCheckLogsPagination(
    pagination.total,
    limit,
    offset,
  );

  const statusBuckets = [
    {
      label: "2xx Sucesso",
      count: insights.statusDistribution["2xx"],
      barColor: "bg-emerald-500",
    },
    {
      label: "3xx Redirecionamento",
      count: insights.statusDistribution["3xx"],
      barColor: "bg-cyan-500",
    },
    {
      label: "4xx Erro Cliente",
      count: insights.statusDistribution["4xx"],
      barColor: "bg-amber-500",
    },
    {
      label: "5xx Erro Servidor",
      count: insights.statusDistribution["5xx"],
      barColor: "bg-rose-500",
    },
    {
      label: "Rede / Timeout (0)",
      count: insights.statusDistribution.network_error,
      barColor: "bg-fuchsia-500",
    },
  ];

  return (
    <div className="space-y-6">
      {/* Filters Bar */}
      <section className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-4">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-white flex items-center gap-2">
              <Filter className="w-4 h-4 text-indigo-400" />
              Histórico & Insights de Checklogs
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Filtre registros persistidos de verificações e analise
              disponibilidade, latência e falhas.
            </p>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={handleResetFilters}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-slate-950 border border-slate-700 text-slate-300 hover:text-white cursor-pointer"
            >
              <RotateCcw className="w-3.5 h-3.5" />
              Limpar Filtros
            </button>
            <button
              type="button"
              onClick={handleRefresh}
              disabled={isFetching}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold bg-indigo-600 hover:bg-indigo-500 text-white cursor-pointer disabled:opacity-50"
            >
              <RefreshCw
                className={`w-3.5 h-3.5 ${isFetching ? "animate-spin" : ""}`}
              />
              Atualizar
            </button>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3">
          <div>
            <label
              htmlFor="filter-target"
              className="block text-[11px] font-medium text-slate-400 mb-1"
            >
              Alvo (target_id)
            </label>
            <select
              id="filter-target"
              value={targetId}
              onChange={(e) => {
                setTargetId(e.target.value);
                setOffset(0);
              }}
              className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs text-white focus:outline-none focus:border-indigo-500"
            >
              <option value="">Todos os alvos</option>
              {targets.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name} ({t.url})
                </option>
              ))}
            </select>
          </div>

          <div>
            <label
              htmlFor="filter-is-up"
              className="block text-[11px] font-medium text-slate-400 mb-1"
            >
              Disponibilidade (is_up)
            </label>
            <select
              id="filter-is-up"
              value={isUpFilter}
              onChange={(e) => {
                setIsUpFilter(e.target.value as "all" | "true" | "false");
                setOffset(0);
              }}
              className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs text-white focus:outline-none focus:border-indigo-500"
            >
              <option value="all">Todos</option>
              <option value="true">Somente UP</option>
              <option value="false">Somente DOWN</option>
            </select>
          </div>

          <div>
            <label
              htmlFor="filter-status-code"
              className="block text-[11px] font-medium text-slate-400 mb-1"
            >
              Código HTTP (status_code)
            </label>
            <input
              id="filter-status-code"
              type="text"
              inputMode="numeric"
              value={statusCodeInput}
              onChange={(e) => {
                setStatusCodeInput(e.target.value);
                setOffset(0);
              }}
              placeholder="Ex: 200, 503, 0"
              className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs font-mono text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
            />
          </div>

          <div>
            <label
              htmlFor="filter-window"
              className="block text-[11px] font-medium text-slate-400 mb-1"
            >
              Janela de Tempo
            </label>
            <select
              id="filter-window"
              value={timePreset}
              onChange={(e) => {
                setTimePreset(e.target.value as TimePreset);
                setOffset(0);
              }}
              className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs text-white focus:outline-none focus:border-indigo-500"
            >
              <option value="1h">Última 1 hora</option>
              <option value="24h">Últimas 24 horas</option>
              <option value="7d">Últimos 7 dias</option>
              <option value="30d">Últimos 30 dias</option>
              <option value="all">Todo o histórico</option>
              <option value="custom">Intervalo personalizado</option>
            </select>
          </div>

          <div>
            <label
              htmlFor="filter-search"
              className="block text-[11px] font-medium text-slate-400 mb-1"
            >
              Busca (nome, URL, erro)
            </label>
            <div className="relative">
              <Search className="w-3.5 h-3.5 text-slate-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
              <input
                id="filter-search"
                type="text"
                value={search}
                onChange={(e) => {
                  setSearch(e.target.value);
                  setOffset(0);
                }}
                placeholder="Buscar logs..."
                className="w-full pl-8 pr-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs text-white placeholder:text-slate-500 focus:outline-none focus:border-indigo-500"
              />
            </div>
          </div>
        </div>

        {timePreset === "custom" && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-slate-800/80">
            <div>
              <label
                htmlFor="filter-from"
                className="block text-[11px] font-medium text-slate-400 mb-1"
              >
                De (from)
              </label>
              <input
                id="filter-from"
                type="datetime-local"
                value={fromLocal}
                onChange={(e) => {
                  setFromLocal(e.target.value);
                  setOffset(0);
                }}
                className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs font-mono text-white focus:outline-none focus:border-indigo-500"
              />
            </div>

            <div>
              <label
                htmlFor="filter-to"
                className="block text-[11px] font-medium text-slate-400 mb-1"
              >
                Até (to)
              </label>
              <input
                id="filter-to"
                type="datetime-local"
                value={toLocal}
                onChange={(e) => {
                  setToLocal(e.target.value);
                  setOffset(0);
                }}
                className="w-full px-3 py-2 rounded-lg bg-slate-950 border border-slate-700 text-xs font-mono text-white focus:outline-none focus:border-indigo-500"
              />
            </div>
          </div>
        )}

        {displayError && (
          <div className="p-3 rounded-lg bg-rose-950/60 border border-rose-800 text-rose-200 text-xs">
            {displayError}
          </div>
        )}
      </section>

      {/* Derived Insights KPI Cards */}
      <section className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-4">
        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4">
          <div className="flex items-center justify-between text-xs text-slate-400 font-medium uppercase tracking-wider">
            <span>Checagens Analisadas</span>
            <BarChart3 className="w-4 h-4 text-indigo-400" />
          </div>
          <div className="text-2xl font-bold text-white mt-1.5">
            {insights.totalChecks}
          </div>
          <div className="text-xs text-slate-400 mt-1">
            Total filtrado no servidor: {pagination.total}
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4">
          <div className="flex items-center justify-between text-xs text-slate-400 font-medium uppercase tracking-wider">
            <span>Disponibilidade (Uptime)</span>
            <CheckCircle2 className="w-4 h-4 text-emerald-400" />
          </div>
          <div className="text-2xl font-bold text-emerald-400 mt-1.5">
            {insights.uptimePercent.toFixed(1)}%
          </div>
          <div className="text-xs text-slate-400 mt-1">
            {insights.upCount} sucesso(s) · {insights.downCount} falha(s)
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4">
          <div className="flex items-center justify-between text-xs text-slate-400 font-medium uppercase tracking-wider">
            <span>Latência Média / P95</span>
            <Gauge className="w-4 h-4 text-amber-400" />
          </div>
          <div className="text-2xl font-bold text-amber-300 mt-1.5 font-mono">
            {insights.avgLatencyMs.toFixed(1)} ms
          </div>
          <div className="text-xs text-slate-400 mt-1 font-mono">
            P95: {insights.p95LatencyMs.toFixed(1)} ms · Máx:{" "}
            {insights.maxLatencyMs.toFixed(1)} ms
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4">
          <div className="flex items-center justify-between text-xs text-slate-400 font-medium uppercase tracking-wider">
            <span>Alvo Mais Rápido (Média)</span>
            <Zap className="w-4 h-4 text-cyan-400" />
          </div>
          {insights.fastestTarget ? (
            <>
              <div
                className="text-lg font-bold text-cyan-300 mt-1.5 truncate"
                title={insights.fastestTarget.name}
              >
                {insights.fastestTarget.name}
              </div>
              <div className="text-xs text-slate-400 mt-1 font-mono">
                {insights.fastestTarget.avgLatencyMs.toFixed(1)} ms ·{" "}
                {insights.fastestTarget.total} checagem(ns)
              </div>
            </>
          ) : (
            <>
              <div className="text-2xl font-bold text-slate-500 mt-1.5">--</div>
              <div className="text-xs text-slate-500 mt-1">
                Sem dados de latência
              </div>
            </>
          )}
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4">
          <div className="flex items-center justify-between text-xs text-slate-400 font-medium uppercase tracking-wider">
            <span>Taxa de Erros</span>
            <AlertTriangle className="w-4 h-4 text-rose-400" />
          </div>
          <div className="text-2xl font-bold text-rose-400 mt-1.5">
            {insights.errorRatePercent.toFixed(1)}%
          </div>
          <div className="text-xs text-slate-400 mt-1">
            Mínima: {insights.minLatencyMs.toFixed(1)} ms
          </div>
        </div>
      </section>

      {/* Distribution & Top Targets Breakdown */}
      <section className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-3">
          <h3 className="text-sm font-semibold text-white">
            Distribuição de Status HTTP
          </h3>
          <div className="space-y-2.5">
            {statusBuckets.map((bucket) => {
              const pct =
                insights.totalChecks > 0
                  ? (bucket.count / insights.totalChecks) * 100
                  : 0;
              return (
                <div key={bucket.label} className="space-y-1">
                  <div className="flex justify-between text-xs">
                    <span className="text-slate-300">{bucket.label}</span>
                    <span className="font-mono text-slate-400">
                      {bucket.count} ({pct.toFixed(0)}%)
                    </span>
                  </div>
                  <div className="h-2 rounded-full bg-slate-950 overflow-hidden border border-slate-800">
                    <div
                      className={`h-full ${bucket.barColor} transition-all`}
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-3">
          <h3 className="text-sm font-semibold text-white">
            Alvos com Mais Falhas
          </h3>
          {insights.topFailingTargets.length === 0 ? (
            <p className="text-xs text-slate-500 py-4">
              Nenhuma falha registrada no conjunto filtrado.
            </p>
          ) : (
            <div className="space-y-2.5">
              {insights.topFailingTargets.map((item) => (
                <div
                  key={item.targetKey}
                  className="flex items-center justify-between gap-2 p-2.5 rounded-lg bg-slate-950/70 border border-slate-800/80 text-xs"
                >
                  <div className="min-w-0">
                    <div className="font-medium text-white truncate">
                      {item.name}
                    </div>
                    <div className="text-[11px] font-mono text-slate-500 truncate">
                      {item.url}
                    </div>
                  </div>
                  <div className="text-right shrink-0">
                    <div className="font-semibold text-rose-400">
                      {item.failures} falha(s)
                    </div>
                    <div className="text-[11px] text-slate-400">
                      {item.uptimePercent.toFixed(0)}% uptime
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-5 space-y-3">
          <h3 className="text-sm font-semibold text-white">
            Alvos Mais Lentos (Média)
          </h3>
          {insights.slowestTargets.length === 0 ? (
            <p className="text-xs text-slate-500 py-4">
              Sem dados de latência disponíveis.
            </p>
          ) : (
            <div className="space-y-2.5">
              {insights.slowestTargets.map((item) => (
                <div
                  key={item.targetKey}
                  className="flex items-center justify-between gap-2 p-2.5 rounded-lg bg-slate-950/70 border border-slate-800/80 text-xs"
                >
                  <div className="min-w-0">
                    <div className="font-medium text-white truncate">
                      {item.name}
                    </div>
                    <div className="text-[11px] font-mono text-slate-500 truncate">
                      {item.total} checagem(ns)
                    </div>
                  </div>
                  <div className="font-mono font-semibold text-amber-300 shrink-0">
                    {item.avgLatencyMs.toFixed(1)} ms
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </section>

      {/* Checklogs Timeline List */}
      <section className="bg-slate-900/80 border border-slate-800 rounded-xl overflow-hidden">
        <div className="px-5 py-4 border-b border-slate-800 flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Clock className="w-4 h-4 text-slate-400" />
            <h3 className="text-sm font-semibold text-white">
              Registros de Verificação ({pagination.total})
            </h3>
          </div>

          <div className="flex items-center gap-2 text-xs text-slate-400">
            <label htmlFor="page-size">Itens por página:</label>
            <select
              id="page-size"
              value={limit}
              onChange={(e) => {
                setLimit(Number.parseInt(e.target.value, 10));
                setOffset(0);
              }}
              className="px-2 py-1 rounded bg-slate-950 border border-slate-700 text-xs text-white"
            >
              <option value={25}>25</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
              <option value={200}>200</option>
            </select>
          </div>
        </div>

        {isFetching ? (
          <div className="p-8 text-center text-sm text-slate-400">
            Carregando registros...
          </div>
        ) : logs.length === 0 ? (
          <div className="p-8 text-center text-sm text-slate-500">
            Nenhum checklog encontrado para os filtros selecionados.
          </div>
        ) : (
          <div className="divide-y divide-slate-800/70">
            {logs.map((log) => (
              <div
                key={log.id}
                className="px-5 py-3.5 flex flex-col md:flex-row md:items-center justify-between gap-3 hover:bg-slate-800/30 transition-colors"
              >
                <div className="flex items-start gap-3 min-w-0">
                  {log.is_up ? (
                    <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
                  ) : (
                    <XCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
                  )}
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className="text-sm font-semibold text-white">
                        {log.name || log.url}
                      </span>
                      <span
                        className={`px-2 py-0.5 rounded-full text-[11px] font-semibold border ${
                          log.is_up
                            ? "bg-emerald-950/60 text-emerald-400 border-emerald-800/70"
                            : "bg-rose-950/60 text-rose-400 border-rose-800/70"
                        }`}
                      >
                        {log.is_up ? "UP" : "DOWN"} · HTTP{" "}
                        {log.status_code > 0 ? log.status_code : "0 (Rede)"}
                      </span>
                    </div>
                    <div className="text-xs font-mono text-slate-400 break-all mt-0.5">
                      {log.url}
                    </div>
                    {log.error && (
                      <div className="text-xs font-mono text-rose-400 mt-1 break-all">
                        {log.error}
                      </div>
                    )}
                  </div>
                </div>

                <div className="flex items-center gap-4 text-xs shrink-0 pl-7 md:pl-0">
                  <span className="font-mono text-amber-300">
                    {formatLatencyMs(log.latency)}
                  </span>
                  <span className="font-mono text-slate-400">
                    {formatDateTime(log.checked_at)}
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Pagination Controls */}
        <div className="px-5 py-3 border-t border-slate-800 flex items-center justify-between text-xs text-slate-400">
          <span>
            Página {currentPage} de {totalPages}
          </span>

          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={currentPage <= 1 || isFetching}
              onClick={() => setOffset(Math.max(0, clampedOffset - limit))}
              className="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-300 hover:text-white cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
            >
              <ChevronLeft className="w-3.5 h-3.5" />
              Anterior
            </button>
            <button
              type="button"
              disabled={currentPage >= totalPages || isFetching}
              onClick={() => setOffset(clampedOffset + limit)}
              className="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg bg-slate-950 border border-slate-700 text-slate-300 hover:text-white cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed"
            >
              Próxima
              <ChevronRight className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </section>
    </div>
  );
}
