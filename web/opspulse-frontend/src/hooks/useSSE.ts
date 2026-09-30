import { useState, useEffect, useCallback, useRef } from "react";
import type { CheckResult, ConnectionStatus, LogMessage } from "@/types/check";

export function useSSE(endpoint = "/api/v1/events") {
  const [targets, setTargets] = useState<CheckResult[]>([]);
  const [status, setStatus] = useState<ConnectionStatus>("connecting");
  const [lastUpdate, setLastUpdate] = useState<string | null>(null);
  const [logs, setLogs] = useState<LogMessage[]>([]);
  const eventSourceRef = useRef<EventSource | null>(null);

  const addLog = useCallback(
    (text: string, type: LogMessage["type"] = "info") => {
      const newLog: LogMessage = {
        id: Math.random().toString(36).substring(2, 9),
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

  const stop = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close();
      eventSourceRef.current = null;
      setStatus("disconnected");
      addLog("Monitoramento pausado pelo usuário.", "warn");
    }
  }, [addLog]);

  const connect = useCallback(() => {
    const es = new EventSource(endpoint);
    eventSourceRef.current = es;

    addLog(`Conectando ao endpoint SSE: ${endpoint}...`, "warn");
    setStatus("connecting");

    es.onopen = () => {
      setStatus("connected");
      addLog("Conexão SSE estabelecida com sucesso!", "success");
    };

    es.onerror = () => {
      setStatus("connecting");
      addLog(
        "Conexão SSE oscilou. Tentando reconectar automaticamente...",
        "warn",
      );
    };

    es.addEventListener("status", (e) => {
      try {
        const data: CheckResult[] = JSON.parse(e.data);
        setTargets(data);
        setLastUpdate(new Date().toLocaleTimeString());
        addLog(
          `Evento [status] recebido: ${data.length} serviços atualizados`,
          "success",
        );
      } catch (err) {
        addLog(`Erro ao parsear dados do evento status: ${err}`, "error");
      }
    });

    es.addEventListener("alert", (e) => {
      try {
        const alertData: CheckResult = JSON.parse(e.data);
        addLog(`⚠️ ALERTA: Serviço indisponível: ${alertData.URL}`, "error");
      } catch (err) {
        addLog(`Erro ao parsear dados do alerta: ${err}`, "error");
      }
    });
  }, [endpoint, addLog]);

  const restart = useCallback(() => {
    stop();
    connect();
  }, [stop, connect]);

  useEffect(() => {
    connect();
  }, [connect]);

  return {
    targets,
    status,
    lastUpdate,
    logs,
    addLog,
    clearLogs,
    stop,
    restart,
  };
}
