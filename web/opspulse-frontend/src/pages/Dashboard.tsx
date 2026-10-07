import { useSSE } from "@/hooks/useSSE";
import { Header } from "@/components/Header";
import { StatCards } from "@/components/StatCards";
import { TargetTable } from "@/components/TargetTable";
import { EventLog } from "@/components/EventLog";

export function DashboardPage() {
  const { targets, status, lastUpdate, logs, clearLogs, stop, restart } =
    useSSE("/api/v1/targets/events");

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 font-sans antialiased p-6">
      <div className="max-w-6xl mx-auto space-y-6">
        <Header status={status} onRestart={restart} onStop={stop} />
        <StatCards targets={targets} lastUpdate={lastUpdate} />
        <TargetTable targets={targets} />
        <EventLog logs={logs} onClear={clearLogs} />
      </div>
    </div>
  );
}
