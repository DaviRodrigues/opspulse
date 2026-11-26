import { Ban, History, LayoutGrid, Radar, RefreshCw } from "lucide-react";
import type { ConnectionStatus } from "@/types/check";

export type AppView = "dashboard" | "checklogs";

interface HeaderProps {
  activeView: AppView;
  onNavigate: (view: AppView) => void;
  status: ConnectionStatus;
  onRestart: () => void;
  onStop: () => void;
}

export function Header({
  activeView,
  onNavigate,
  status,
  onRestart,
  onStop,
}: HeaderProps) {
  return (
    <header className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 pb-5 border-b border-slate-800">
      <div className="flex flex-col sm:flex-row sm:items-center gap-4 sm:gap-6">
        <div>
          <div className="flex items-center gap-3">
            <Radar className="w-8 h-8 text-indigo-400" />
            <h1 className="text-2xl font-bold tracking-tight text-white">
              OpsPulse
            </h1>
          </div>
          <p className="text-xs text-slate-400 mt-1">
            Monitoramento em Tempo Real via SSE & Histórico de Verificações
          </p>
        </div>

        <nav
          aria-label="Navegação principal"
          className="inline-flex items-center p-1 rounded-xl bg-slate-900 border border-slate-800 self-start"
        >
          <button
            type="button"
            onClick={() => onNavigate("dashboard")}
            className={`inline-flex items-center gap-2 px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-colors cursor-pointer ${
              activeView === "dashboard"
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white"
            }`}
          >
            <LayoutGrid className="w-3.5 h-3.5" />
            Dashboard
          </button>

          <button
            type="button"
            onClick={() => onNavigate("checklogs")}
            className={`inline-flex items-center gap-2 px-3.5 py-1.5 rounded-lg text-xs font-semibold transition-colors cursor-pointer ${
              activeView === "checklogs"
                ? "bg-indigo-600 text-white shadow-sm"
                : "text-slate-400 hover:text-white"
            }`}
          >
            <History className="w-3.5 h-3.5" />
            Checklogs
          </button>
        </nav>
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        {/* SSE Status Badge */}
        {status === "connected" && (
          <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full text-xs font-semibold bg-emerald-950/60 border border-emerald-800 text-emerald-400">
            <span className="w-2 h-2 rounded-full bg-emerald-400 live-dot" />
            Conectado ao SSE
          </div>
        )}
        {status === "connecting" && (
          <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full text-xs font-semibold bg-amber-950/60 border border-amber-800 text-amber-400">
            <span className="w-2 h-2 rounded-full bg-amber-400 live-dot" />
            Reconectando...
          </div>
        )}
        {status === "disconnected" && (
          <div className="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full text-xs font-semibold bg-rose-950/60 border border-rose-800 text-rose-400">
            <span className="w-2 h-2 rounded-full bg-rose-400" />
            Desconectado
          </div>
        )}

        <button
          type="button"
          onClick={onRestart}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-slate-900 border border-slate-700 text-slate-300 hover:text-white hover:border-slate-600 transition-colors cursor-pointer"
        >
          <RefreshCw className="w-3.5 h-3.5 text-blue-400" />
          Recarregar Monitor
        </button>

        <button
          type="button"
          onClick={onStop}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium bg-slate-900 border border-slate-700 text-slate-300 hover:text-rose-400 hover:border-rose-900 transition-colors cursor-pointer"
        >
          <Ban className="w-3.5 h-3.5 text-rose-400" />
          Parar Monitor
        </button>
      </div>
    </header>
  );
}
