import { useCallback, useEffect, useState } from "react";
import { Header, type AppView } from "@/components/Header";
import { useSSE } from "@/hooks/useSSE";
import { CheckLogsPage } from "@/pages/CheckLogsPage";
import { DashboardPage } from "@/pages/Dashboard";

function parseHashRoute(): { view: AppView; targetId: string } {
  const hash = window.location.hash || "";
  if (hash.startsWith("#/check-logs")) {
    const queryIndex = hash.indexOf("?");
    if (queryIndex !== -1) {
      const params = new URLSearchParams(hash.slice(queryIndex + 1));
      return {
        view: "checklogs",
        targetId: params.get("target_id") ?? "",
      };
    }
    return { view: "checklogs", targetId: "" };
  }
  return { view: "dashboard", targetId: "" };
}

export function App() {
  const initialRoute = parseHashRoute();
  const [activeView, setActiveView] = useState<AppView>(initialRoute.view);
  const [selectedTargetId, setSelectedTargetId] = useState(
    initialRoute.targetId,
  );

  const {
    targets,
    hasReceivedStatus,
    status,
    lastUpdate,
    logs,
    clearLogs,
    stop,
    restart,
  } = useSSE();

  useEffect(() => {
    const handleHashChange = () => {
      const next = parseHashRoute();
      setActiveView(next.view);
      setSelectedTargetId(next.targetId);
    };
    window.addEventListener("hashchange", handleHashChange);
    return () => {
      window.removeEventListener("hashchange", handleHashChange);
    };
  }, []);

  const navigateTo = useCallback((view: AppView, targetId = "") => {
    setActiveView(view);
    setSelectedTargetId(targetId);
    if (view === "checklogs") {
      window.location.hash = targetId
        ? `#/check-logs?target_id=${encodeURIComponent(targetId)}`
        : "#/check-logs";
    } else {
      window.location.hash = "#/";
    }
  }, []);

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 font-sans antialiased p-6">
      <div className="max-w-7xl mx-auto space-y-6">
        <Header
          activeView={activeView}
          onNavigate={(view) => navigateTo(view)}
          status={status}
          onRestart={restart}
          onStop={stop}
        />

        {activeView === "dashboard" ? (
          <DashboardPage
            liveTargets={targets}
            hasReceivedStatus={hasReceivedStatus}
            lastUpdate={lastUpdate}
            logs={logs}
            onClearLogs={clearLogs}
            onViewCheckLogs={(targetId) => navigateTo("checklogs", targetId)}
          />
        ) : (
          <CheckLogsPage
            key={selectedTargetId || "all"}
            initialTargetId={selectedTargetId}
          />
        )}
      </div>
    </div>
  );
}

export default App;
