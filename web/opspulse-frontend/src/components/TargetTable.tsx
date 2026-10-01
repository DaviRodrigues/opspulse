import type { CheckResult } from "@/types/check";
import { Eye } from "lucide-react";

interface TargetTableProps {
  targets: CheckResult[];
}

export function TargetTable({ targets }: TargetTableProps) {
  return (
    <div className="bg-slate-900/80 border border-slate-800 rounded-xl overflow-hidden shadow-xl">
      <div className="p-4 border-b border-slate-800 flex justify-between items-center">
        <h2 className="text-base font-semibold text-white flex items-center gap-2">
          <Eye /> Serviços Monitorados
        </h2>
        <span className="text-xs text-slate-400">
          Atualizações automáticas em tempo real
        </span>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm text-slate-300">
          <thead className="bg-slate-950/60 text-xs uppercase text-slate-400 border-b border-slate-800">
            <tr>
              <th className="px-6 py-3">Status</th>
              <th className="px-6 py-3">Serviço / URL</th>
              <th className="px-6 py-3">Código HTTP</th>
              <th className="px-6 py-3">Latência</th>
              <th className="px-6 py-3">Diagnóstico</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800/60">
            {targets.length === 0 ? (
              <tr>
                <td
                  colSpan={5}
                  className="px-6 py-8 text-center text-slate-500"
                >
                  Aguardando primeiro evento do servidor...
                </td>
              </tr>
            ) : (
              targets.map((t, idx) => {
                const latencyText = t.Latency
                  ? `${(t.Latency / 1000000).toFixed(2)}ms`
                  : "--";
                const codeText = t.StatusCode > 0 ? t.StatusCode : "N/A";

                return (
                  <tr
                    key={`${t.URL}-${idx}`}
                    className="hover:bg-slate-800/40 transition-colors"
                  >
                    <td className="px-6 py-4">
                      {t.IsUp ? (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-emerald-900/40 text-emerald-400 border border-emerald-800/60">
                          🟢 UP
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium bg-rose-900/40 text-rose-400 border border-rose-800/60">
                          🔴 DOWN
                        </span>
                      )}
                    </td>
                    <td className="px-6 py-4 font-medium text-white break-all">
                      {t.Name ? (
                        <div>
                          <div className="text-slate-100 font-semibold">
                            {t.Name}
                          </div>
                          <div className="text-xs text-slate-400 font-mono">
                            {t.URL}
                          </div>
                        </div>
                      ) : (
                        t.URL
                      )}
                    </td>
                    <td className="px-6 py-4 font-mono text-xs">{codeText}</td>
                    <td className="px-6 py-4 font-mono text-xs text-amber-300/90">
                      {latencyText}
                    </td>
                    <td className="px-6 py-4">
                      {t.Error ? (
                        <span className="text-rose-400 font-mono text-xs break-all">
                          {t.Error}
                        </span>
                      ) : (
                        <span className="text-slate-500 text-xs">
                          Operacional
                        </span>
                      )}
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
