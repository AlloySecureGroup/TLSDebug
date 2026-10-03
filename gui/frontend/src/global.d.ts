export {};

declare global {
  interface Settings {
    proxyPort: number;
    monitorPort: number;
    dataDir: string;
    proxyBinary: string;
  }

  interface AppState {
    running: boolean;
    settings: Settings;
    caPath: string;
    binaryPath: string;
  }

  interface TrafficEntry {
    id: number;
    timestamp: string;
    method: string;
    url: string;
    host: string;
    path: string;
    statusCode: number;
    statusText: string;
    requestHeaders: Record<string, string[]> | null;
    responseHeaders: Record<string, string[]> | null;
    requestBody: string;
    responseBody: string;
    contentType: string;
    duration: number;
    tlsVersion: string;
    clientAddr: string;
  }

  interface Window {
    go: {
      main: {
        App: {
          GetState(): Promise<AppState>;
          UpdateSettings(settings: Settings): Promise<AppState>;
          StartProxy(): Promise<AppState>;
          StopProxy(): Promise<AppState>;
          GetSessions(): Promise<TrafficEntry[]>;
          ClearSessions(): Promise<void>;
          SaveSessions(entries: TrafficEntry[]): Promise<string>;
          ImportSessions(): Promise<TrafficEntry[]>;
          InstallRootCA(): Promise<string>;
        };
      };
    };
    runtime: {
      EventsOn(event: string, callback: (...args: unknown[]) => void): () => void;
    };
  }
}
