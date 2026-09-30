import type { CheckResult } from "@/types/check";

interface StatCardsProps {
  targets: CheckResult[];
  lastUpdate: string | null;
}

export function StatCards({ targets, lastUpdate }: StatCardsProps) {
  const total = targets.length;
  const up = targets.filter((t) => t.IsUp).length;
  const down = total - up;

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4 shadow-sm">
        <div className="text-xs text-slate-400 font-medium uppercase tracking-wider">
          Total de Alvos
        </div>
        <div className="text-2xl font-bold text-white mt-1">{total}</div>
      </div>

      <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4 shadow-sm">
        <div className="text-xs text-slate-400 font-medium uppercase tracking-wider">
          Serviços Online
        </div>
        <div className="text-2xl font-bold text-emerald-400 mt-1">{up}</div>
      </div>

      <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4 shadow-sm">
        <div className="text-xs text-slate-400 font-medium uppercase tracking-wider">
          Serviços com Falha
        </div>
        <div className="text-2xl font-bold text-rose-400 mt-1">{down}</div>
      </div>

      <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4 shadow-sm">
        <div className="text-xs text-slate-400 font-medium uppercase tracking-wider">
          Última Atualização
        </div>
        <div className="text-sm font-mono text-slate-300 mt-2">
          {lastUpdate ?? "Aguardando..."}
        </div>
      </div>
    </div>
  );
}
