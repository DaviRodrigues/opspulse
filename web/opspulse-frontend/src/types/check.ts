export interface CheckResult {
  Name: string;
  URL: string;
  StatusCode: number;
  Latency: number; // Nanosegundos vindos do Go
  IsUp: boolean;
  Error?: string;
}

export type ConnectionStatus = "connected" | "connecting" | "disconnected";

export interface LogMessage {
  id: string;
  timestamp: string;
  text: string;
  type: "info" | "success" | "warn" | "error";
}
