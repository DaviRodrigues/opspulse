import { Radar, RefreshCw, Ban } from 'lucide-react'
import type { ConnectionStatus } from '@/types/check'

interface HeaderProps {
  status: ConnectionStatus
  onRestart: () => void
  onStop: () => void
}

export function Header({ status, onRestart, onStop }: HeaderProps) {
  return (
    <header className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-6 border-b border-slate-800">
      <div>
        <div className="flex items-center gap-3">
          <Radar className="w-8 h-8 text-indigo-400" />
          <h1 className="text-2xl font-bold tracking-tight text-white">
            OpsPulse Dashboard
          </h1>
        </div>
        <p className="text-sm text-slate-400 mt-1">
          Monitoramento em Tempo Real via Server-Sent Events (SSE)
        </p>
      </div>

      <div className="flex items-center gap-3">
        {/* Status Badge */}
        {status === 'connected' && (
          <div className="inline-flex items-center gap-2.5 px-4 py-2 rounded-full text-xs font-semibold bg-emerald-950/60 border border-emerald-800 text-emerald-400">
            <span className="w-2.5 h-2.5 rounded-full bg-emerald-400 live-dot" />
            Conectado ao SSE
          </div>
        )}
        {status === 'connecting' && (
          <div className="inline-flex items-center gap-2.5 px-4 py-2 rounded-full text-xs font-semibold bg-amber-950/60 border border-amber-800 text-amber-400">
            <span className="w-2.5 h-2.5 rounded-full bg-amber-400 live-dot" />
            Reconectando...
          </div>
        )}
        {status === 'disconnected' && (
          <div className="inline-flex items-center gap-2.5 px-4 py-2 rounded-full text-xs font-semibold bg-rose-950/60 border border-rose-800 text-rose-400">
            <span className="w-2.5 h-2.5 rounded-full bg-rose-400" />
            Desconectado
          </div>
        )}

        {/* Action Buttons */}
        <button
          onClick={onRestart}
          className="inline-flex items-center gap-2 px-3.5 py-2 rounded-lg text-xs font-medium bg-slate-900 border border-slate-700 text-slate-300 hover:text-white hover:border-slate-600 transition-colors cursor-pointer"
        >
          <RefreshCw className="w-4 h-4 text-blue-400" />
          Recarregar Monitor
        </button>

        <button
          onClick={onStop}
          className="inline-flex items-center gap-2 px-3.5 py-2 rounded-lg text-xs font-medium bg-slate-900 border border-slate-700 text-slate-300 hover:text-rose-400 hover:border-rose-900 transition-colors cursor-pointer"
        >
          <Ban className="w-4 h-4 text-rose-400" />
          Parar Monitor
        </button>
      </div>
    </header>
  )
}

