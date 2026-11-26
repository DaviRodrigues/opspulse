import { useEffect, useState } from "react";
import { StatCards } from "@/components/StatCards";
import { TargetCards } from "@/components/TargetCards";
import { TargetModal } from "@/components/TargetModal";
import { EventLog } from "@/components/EventLog";
import {
  deleteTarget,
  extractApiErrorMessage,
  getTargetsStatus,
  listTargets,
  patchTarget,
} from "@/lib/api";
import {
  normalizeTargetMatchKey,
  pruneDeletedTargetStatus,
  resolveEffectiveLiveResults,
} from "@/lib/targetStatus";
import type { CheckResult, LogMessage, Target } from "@/types/check";

interface DashboardPageProps {
  liveTargets: CheckResult[];
  hasReceivedStatus?: boolean;
  lastUpdate: string | null;
  logs: LogMessage[];
  onClearLogs: () => void;
  onViewCheckLogs: (targetId: string) => void;
}

export function DashboardPage({
  liveTargets,
  hasReceivedStatus = false,
  lastUpdate,
  logs,
  onClearLogs,
  onViewCheckLogs,
}: DashboardPageProps) {
  const [targets, setTargets] = useState<Target[]>([]);
  const [initialStatus, setInitialStatus] = useState<CheckResult[]>([]);
  const [deletedOverlay, setDeletedOverlay] = useState<{
    urls: string[];
    liveRef: CheckResult[];
  }>({ urls: [], liveRef: liveTargets });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busyTargetId, setBusyTargetId] = useState<string | null>(null);

  const [modalOpen, setModalOpen] = useState(false);
  const [editingTarget, setEditingTarget] = useState<Target | null>(null);

  const [refreshTick, setRefreshTick] = useState(0);

  useEffect(() => {
    let active = true;

    Promise.all([
      listTargets({ limit: 200, offset: 0 }),
      getTargetsStatus().catch(() => [] as CheckResult[]),
    ])
      .then(([targetsPage, statusSnapshot]) => {
        if (!active) {
          return;
        }
        setTargets(targetsPage.data);
        setInitialStatus(statusSnapshot);
        setError(null);
      })
      .catch((err) => {
        if (!active) {
          return;
        }
        setError(
          extractApiErrorMessage(err, "Falha ao carregar alvos monitorados."),
        );
      })
      .finally(() => {
        if (active) {
          setLoading(false);
        }
      });

    return () => {
      active = false;
    };
  }, [refreshTick]);

  const handleRefresh = () => {
    setLoading(true);
    setError(null);
    setRefreshTick((prev) => prev + 1);
  };

  const activeDeletedUrls =
    deletedOverlay.liveRef === liveTargets ? deletedOverlay.urls : [];

  const effectiveLiveResults = resolveEffectiveLiveResults({
    hasReceivedStatus,
    liveTargets,
    initialStatus,
    deletedUrls: activeDeletedUrls,
  });

  const handleCreate = () => {
    setEditingTarget(null);
    setModalOpen(true);
  };

  const handleEdit = (target: Target) => {
    setEditingTarget(target);
    setModalOpen(true);
  };

  const handleSaved = (saved: Target, mode: "create" | "update") => {
    const savedKey = normalizeTargetMatchKey(saved.url);
    setDeletedOverlay((prev) => {
      if (prev.liveRef !== liveTargets || prev.urls.length === 0) {
        return prev;
      }
      const nextUrls = prev.urls.filter(
        (u) => normalizeTargetMatchKey(u) !== savedKey,
      );
      return nextUrls.length === prev.urls.length
        ? prev
        : { urls: nextUrls, liveRef: liveTargets };
    });
    setTargets((prev) => {
      if (mode === "create") {
        return [saved, ...prev];
      }
      return prev.map((item) => (item.id === saved.id ? saved : item));
    });
  };

  const handleDeleted = (deletedId: string) => {
    const deletedTarget = targets.find((item) => item.id === deletedId);
    setTargets((prev) => prev.filter((item) => item.id !== deletedId));
    if (deletedTarget?.url) {
      setInitialStatus((prev) =>
        pruneDeletedTargetStatus(prev, deletedTarget.url),
      );
      setDeletedOverlay((prev) => ({
        urls: [
          ...(prev.liveRef === liveTargets ? prev.urls : []),
          deletedTarget.url,
        ],
        liveRef: liveTargets,
      }));
    }
  };

  const handleToggleEnabled = async (target: Target) => {
    setBusyTargetId(target.id);
    setError(null);
    try {
      const updated = await patchTarget(target.id, {
        enabled: !target.enabled,
      });
      setTargets((prev) =>
        prev.map((item) => (item.id === updated.id ? updated : item)),
      );
    } catch (err) {
      setError(
        extractApiErrorMessage(
          err,
          `Não foi possível alterar o status de "${target.name}".`,
        ),
      );
    } finally {
      setBusyTargetId(null);
    }
  };

  const handleDeleteFromCard = async (target: Target) => {
    setBusyTargetId(target.id);
    setError(null);
    try {
      await deleteTarget(target.id);
      handleDeleted(target.id);
    } catch (err) {
      setError(
        extractApiErrorMessage(
          err,
          `Não foi possível excluir o alvo "${target.name}".`,
        ),
      );
    } finally {
      setBusyTargetId(null);
    }
  };

  return (
    <div className="space-y-6">
      <StatCards targets={effectiveLiveResults} lastUpdate={lastUpdate} />

      <TargetCards
        targets={targets}
        liveResults={effectiveLiveResults}
        loading={loading}
        error={error}
        busyTargetId={busyTargetId}
        onCreate={handleCreate}
        onEdit={handleEdit}
        onToggleEnabled={(t) => void handleToggleEnabled(t)}
        onDelete={(t) => void handleDeleteFromCard(t)}
        onViewCheckLogs={onViewCheckLogs}
        onRefresh={handleRefresh}
      />

      <EventLog logs={logs} onClear={onClearLogs} />

      {modalOpen && (
        <TargetModal
          key={editingTarget?.id ?? "new"}
          isOpen={modalOpen}
          target={editingTarget}
          onClose={() => setModalOpen(false)}
          onSaved={handleSaved}
          onDeleted={handleDeleted}
        />
      )}
    </div>
  );
}
