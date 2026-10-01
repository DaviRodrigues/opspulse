import { useEffect, useRef } from "react";
import { ScrollText, Trash2 } from "lucide-react";
import type { LogMessage } from "@/types/check";

interface EventLogProps {
  logs: LogMessage[];
  onClear: () => void;
}

export function EventLog({ logs, onClear }: EventLogProps) {
  const logContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
    }
  }, [logs]);

  return (
    <div className="bg-slate-900/80 border border-slate-800 rounded-xl p-4 shadow-xl">
      <div className="flex justify-between items-center mb-3">
        <div className="flex items-center gap-2">
          <ScrollText className="w-4 h-4 text-slate-400" />
          <h3 className="text-sm font-semibold text-slate-300">
            Log de Eventos SSE (Raw Stream)
          </h3>
        </div>

        <button
          onClick={onClear}
          className="inline-flex items-center gap-1.5 text-xs text-slate-400 hover:text-slate-200 transition-colors cursor-pointer"
        >
          <Trash2 className="w-3.5 h-3.5" />
          Limpar logs
        </button>
      </div>

      <div
        ref={logContainerRef}
        className="bg-slate-950 font-mono text-xs text-slate-400 p-3 rounded-lg h-36 overflow-y-auto space-y-1.5 border border-slate-800/80 scroll-smooth"
      >
        {logs.length === 0 ? (
          <div className="text-slate-600">
            [Sistema] Nenhum log registrado. Aguardando eventos...
          </div>
        ) : (
          logs.map((log) => {
            let color = "text-slate-400";
            if (log.type === "success") color = "text-emerald-400";
            if (log.type === "error") color = "text-rose-400";
            if (log.type === "warn") color = "text-amber-400";

            return (
              <div key={log.id} className={color}>
                <span className="text-slate-500">[{log.timestamp}]</span>{" "}
                {log.text}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
