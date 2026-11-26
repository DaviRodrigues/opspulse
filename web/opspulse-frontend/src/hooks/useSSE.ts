import { useState, useEffect, useCallback, useRef } from "react";
import { getSSEEndpointUrl } from "@/lib/api";
import type { CheckResult, ConnectionStatus, LogMessage } from "@/types/check";

let logSequence = 0;

function createLogId(): string {
  logSequence += 1;
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `${crypto.randomUUID()}-${logSequence}`;
  }
  return `${Date.now()}-${logSequence}`;
}

export function useSSE(endpoint = getSSEEndpointUrl()) {
  const [targets, setTargets] = useState<CheckResult[]>([]);
  const [hasReceivedStatus, setHasReceivedStatus] = useState(false);
  const [status, setStatus] = useState<ConnectionStatus>("connecting");
  const [lastUpdate, setLastUpdate] = useState<string | null>(null);
  const [logs, setLogs] = useState<LogMessage[]>([]);
  const eventSourceRef = useRef<EventSource | null>(null);
  const mountedRef = useRef(true);

  const addLog = useCallback(
    (text: string, type: LogMessage["type"] = "info") => {
      if (!mountedRef.current) {
        return;
      }
      const newLog: LogMessage = {
        id: createLogId(),
        timestamp: new Date().toLocaleTimeString(),
        text,
        type,
      };
      setLogs((prev) => [...prev.slice(-100), newLog]);
    },
    [],
  );

  const clearLogs = useCallback(() => {
    setLogs([]);
  }, []);

  const closeEventSource = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
    }
  }, []);

  const stop = useCallback(() => {
    if (eventSourceRef.current) {
      closeEventSource();
      if (mountedRef.current) {
        setStatus("disconnected");
        addLog("Monitoramento pausado pelo usuário.", "warn");
      }
    }
  }, [addLog, closeEventSource]);

  const connect = useCallback(() => {
    closeEventSource();

    const es = new EventSource(endpoint);
    eventSourceRef.current = es;

    addLog(`Conectando ao endpoint SSE: ${endpoint}...`, "warn");
    if (mountedRef.current) {
      setStatus("connecting");
    }

    es.onopen = () => {
      if (!mountedRef.current || eventSourceRef.current !== es) {
        return;
      }
      setStatus("connected");
      addLog("Conexão SSE estabelecida com sucesso!", "success");
    };

    es.onerror = () => {
      if (!mountedRef.current || eventSourceRef.current !== es) {
        return;
      }
      if (es.readyState === EventSource.CLOSED) {
        setStatus("disconnected");
        addLog("Conexão SSE encerrada pelo servidor.", "error");
      } else {
        setStatus("connecting");
        addLog(
          "Conexão SSE oscilou. Tentando reconectar automaticamente...",
          "warn",
        );
      }
    };

    es.addEventListener("status", (e) => {
      if (!mountedRef.current || eventSourceRef.current !== es) {
        return;
      }
      try {
        const data: CheckResult[] = JSON.parse(e.data);
        setTargets(Array.isArray(data) ? data : []);
        setHasReceivedStatus(true);
        setLastUpdate(new Date().toLocaleTimeString());
        addLog(
          `Evento [status] recebido: ${Array.isArray(data) ? data.length : 0} serviços atualizados`,
          "success",
        );
      } catch (err) {
        addLog(`Erro ao parsear dados do evento status: ${err}`, "error");
      }
    });

    es.addEventListener("alert", (e) => {
      if (!mountedRef.current || eventSourceRef.current !== es) {
        return;
      }
      try {
        const alertData: CheckResult = JSON.parse(e.data);
        addLog(`⚠️ ALERTA: Serviço indisponível: ${alertData.URL}`, "error");
      } catch (err) {
        addLog(`Erro ao parsear dados do alerta: ${err}`, "error");
      }
    });
  }, [endpoint, addLog, closeEventSource]);

  const restart = useCallback(() => {
    closeEventSource();
    connect();
  }, [closeEventSource, connect]);

  useEffect(() => {
    mountedRef.current = true;
    connect();
    return () => {
      mountedRef.current = false;
      closeEventSource();
    };
  }, [connect, closeEventSource]);

  return {
    targets,
    hasReceivedStatus,
    status,
    lastUpdate,
    logs,
    addLog,
    clearLogs,
    stop,
    restart,
  };
}
