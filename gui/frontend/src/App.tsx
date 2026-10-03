import { useCallback, useEffect, useMemo, useState } from "react";

type InspectorTab = "headers" | "request" | "response" | "raw";
type SessionSource = "live" | "imported";

const emptySettings: Settings = {
  proxyPort: 8080,
  monitorPort: 4040,
  dataDir: "",
  proxyBinary: "",
};

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  return String(error);
}

function formatHeaders(headers: Record<string, string[]> | null): string {
  if (!headers) return "(none)";
  return Object.entries(headers)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, values]) => `${name}: ${values.join(", ")}`)
    .join("\n");
}

function statusClass(code: number): string {
  if (code >= 500) return "status-5xx";
  if (code >= 400) return "status-4xx";
  if (code >= 300) return "status-3xx";
  if (code >= 200) return "status-2xx";
  return "";
}

function App() {
  const [state, setState] = useState<AppState | null>(null);
  const [sessions, setSessions] = useState<TrafficEntry[]>([]);
  const [source, setSource] = useState<SessionSource>("live");
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [filter, setFilter] = useState("");
  const [tab, setTab] = useState<InspectorTab>("headers");
  const [notice, setNotice] = useState("Ready");
  const [busy, setBusy] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [draftSettings, setDraftSettings] = useState<Settings>(emptySettings);
  const [logLines, setLogLines] = useState<string[]>([]);

  const backend = window.go?.main?.App;

  const refreshSessions = useCallback(async () => {
    if (!backend || source !== "live") return;
    try {
      const entries = await backend.GetSessions();
      setSessions(entries ?? []);
      setNotice(`Captured ${entries?.length ?? 0} sessions`);
    } catch {
      if (state?.running) setNotice("Waiting for the proxy monitor…");
    }
  }, [backend, source, state?.running]);

  useEffect(() => {
    if (!backend) {
      setNotice("Run this interface through Wails to connect to the Go backend");
      return;
    }
    backend
      .GetState()
      .then((next) => {
        setState(next);
        setDraftSettings(next.settings);
      })
      .catch((error) => setNotice(errorMessage(error)));

    const offLog = window.runtime?.EventsOn("proxy:log", (...args) => {
      const line = String(args[0] ?? "");
      setLogLines((current) => [...current.slice(-199), line]);
      setNotice(line);
    });
    const offState = window.runtime?.EventsOn("proxy:state", (...args) => {
      const running = Boolean(args[0]);
      setState((current) => (current ? { ...current, running } : current));
    });
    return () => {
      offLog?.();
      offState?.();
    };
  }, [backend]);

  useEffect(() => {
    if (source !== "live") return;
    void refreshSessions();
    const timer = window.setInterval(refreshSessions, state?.running ? 1000 : 3000);
    return () => window.clearInterval(timer);
  }, [refreshSessions, source, state?.running]);

  const filteredSessions = useMemo(() => {
    const query = filter.trim().toLowerCase();
    if (!query) return sessions;
    return sessions.filter((entry) =>
      [entry.method, entry.host, entry.url, entry.statusCode, entry.contentType]
        .join(" ")
        .toLowerCase()
        .includes(query),
    );
  }, [filter, sessions]);

  const selected = useMemo(
    () => sessions.find((entry) => entry.id === selectedID) ?? null,
    [selectedID, sessions],
  );

  async function runAction(action: () => Promise<void>) {
    if (!backend || busy) return;
    setBusy(true);
    try {
      await action();
    } catch (error) {
      setNotice(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }

  function startProxy() {
    void runAction(async () => {
      const next = await backend.StartProxy();
      setState(next);
      setSource("live");
      setNotice(`Proxy listening on 127.0.0.1:${next.settings.proxyPort}`);
    });
  }

  function stopProxy() {
    void runAction(async () => {
      const next = await backend.StopProxy();
      setState(next);
      setNotice("Proxy stopped");
    });
  }

  function installCA() {
    void runAction(async () => {
      setNotice(await backend.InstallRootCA());
    });
  }

  function saveSessions() {
    void runAction(async () => {
      const path = await backend.SaveSessions(sessions);
      if (path) setNotice(`Saved ${sessions.length} sessions to ${path}`);
    });
  }

  function importSessions() {
    void runAction(async () => {
      const imported = await backend.ImportSessions();
      if (!imported) return;
      setSessions(imported);
      setSource("imported");
      setSelectedID(imported[0]?.id ?? null);
      setNotice(`Imported ${imported.length} sessions for review`);
    });
  }

  function clearSessions() {
    void runAction(async () => {
      if (source === "live") await backend.ClearSessions();
      setSessions([]);
      setSelectedID(null);
      setNotice(source === "live" ? "Captured sessions cleared" : "Imported review closed");
    });
  }

  function switchToLive() {
    setSource("live");
    setSessions([]);
    setSelectedID(null);
    setNotice("Showing live capture");
    void refreshSessions();
  }

  function saveSettings() {
    void runAction(async () => {
      const next = await backend.UpdateSettings({
        ...draftSettings,
        proxyPort: Number(draftSettings.proxyPort),
        monitorPort: Number(draftSettings.monitorPort),
      });
      setState(next);
      setDraftSettings(next.settings);
      setShowSettings(false);
      setNotice("Settings saved");
    });
  }

  const requestRaw = selected
    ? `${selected.method} ${selected.url}\n${formatHeaders(selected.requestHeaders)}\n\n${selected.requestBody || ""}`
    : "";
  const responseRaw = selected
    ? `${selected.statusText || selected.statusCode}\n${formatHeaders(selected.responseHeaders)}\n\n${selected.responseBody || ""}`
    : "";

  return (
    <div className="app-shell">
      <header className="menu-bar">
        <div className="brand-mark">T</div>
        <strong>TLSDebug</strong>
        <nav aria-label="Application menu">
          <span>File</span>
          <span>Edit</span>
          <span>Rules</span>
          <span>Tools</span>
          <span>Help</span>
        </nav>
        <div className={`capture-indicator ${state?.running ? "online" : ""}`}>
          <i />
          {state?.running ? "Capturing" : "Offline"}
        </div>
      </header>

      <section className="toolbar" aria-label="Capture controls">
        <button className="primary" disabled={busy || state?.running} onClick={startProxy}>
          <span className="tool-icon">▶</span> Start
        </button>
        <button disabled={busy || !state?.running} onClick={stopProxy}>
          <span className="tool-icon">■</span> Stop
        </button>
        <div className="toolbar-separator" />
        <button disabled={busy} onClick={installCA}>
          <span className="tool-icon">◆</span> Install Root CA
        </button>
        <button disabled={busy || sessions.length === 0} onClick={saveSessions}>
          <span className="tool-icon">▣</span> Save Sessions
        </button>
        <button disabled={busy} onClick={importSessions}>
          <span className="tool-icon">↥</span> Import
        </button>
        <button disabled={busy || sessions.length === 0} onClick={clearSessions}>
          <span className="tool-icon">×</span> Clear
        </button>
        <div className="toolbar-spacer" />
        {source === "imported" && (
          <button className="source-button" onClick={switchToLive}>
            Return to live capture
          </button>
        )}
        <button onClick={() => setShowSettings(true)}>
          <span className="tool-icon">⚙</span> Settings
        </button>
      </section>

      <section className="filter-bar">
        <label htmlFor="session-filter">Filter</label>
        <input
          id="session-filter"
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder="Method, host, URL, status, or content type"
        />
        <span className="source-badge">{source === "live" ? "LIVE" : "IMPORTED"}</span>
        <span>{filteredSessions.length} of {sessions.length} sessions</span>
      </section>

      <main className="workspace">
        <section className="session-pane" aria-label="Captured sessions">
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th className="number-column">#</th>
                  <th>Result</th>
                  <th>Protocol</th>
                  <th>Host</th>
                  <th>URL</th>
                  <th>Body</th>
                  <th>Type</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {filteredSessions.map((entry) => (
                  <tr
                    key={`${source}-${entry.id}`}
                    className={entry.id === selectedID ? "selected" : ""}
                    onClick={() => setSelectedID(entry.id)}
                  >
                    <td className="number-column">{entry.id}</td>
                    <td className={statusClass(entry.statusCode)}>{entry.statusCode || "—"}</td>
                    <td>{entry.tlsVersion || "HTTPS"}</td>
                    <td title={entry.host}>{entry.host}</td>
                    <td title={entry.url}>{entry.path || "/"}</td>
                    <td>{entry.responseBody?.length ?? 0}</td>
                    <td title={entry.contentType}>{entry.contentType?.split(";")[0] || "—"}</td>
                    <td>{entry.duration ? `${(entry.duration / 1_000_000).toFixed(1)} ms` : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {filteredSessions.length === 0 && (
              <div className="empty-state">
                <div className="empty-icon">⇄</div>
                <strong>{source === "live" ? "No traffic captured" : "No matching imported sessions"}</strong>
                <span>
                  {source === "live"
                    ? "Start the proxy and configure a client to use the displayed proxy address."
                    : "Change the filter or import another session file."}
                </span>
              </div>
            )}
          </div>
        </section>

        <section className="inspector-pane" aria-label="Session inspectors">
          <div className="inspector-title">
            <div>
              <strong>Inspectors</strong>
              <span>{selected ? selected.url : "Select a session"}</span>
            </div>
            {selected && <span className={`status-pill ${statusClass(selected.statusCode)}`}>{selected.statusText}</span>}
          </div>
          <div className="tabs" role="tablist">
            {(["headers", "request", "response", "raw"] as InspectorTab[]).map((name) => (
              <button
                key={name}
                role="tab"
                aria-selected={tab === name}
                className={tab === name ? "active" : ""}
                onClick={() => setTab(name)}
              >
                {name[0].toUpperCase() + name.slice(1)}
              </button>
            ))}
          </div>
          <div className="inspector-content">
            {!selected && <div className="inspector-placeholder">Choose a captured session to inspect its request and response.</div>}
            {selected && tab === "headers" && (
              <div className="split-inspector">
                <InspectorBlock title="Request headers" value={formatHeaders(selected.requestHeaders)} />
                <InspectorBlock title="Response headers" value={formatHeaders(selected.responseHeaders)} />
              </div>
            )}
            {selected && tab === "request" && <CodeView value={selected.requestBody || "(request body not captured)"} />}
            {selected && tab === "response" && <CodeView value={selected.responseBody || "(empty response body)"} />}
            {selected && tab === "raw" && (
              <div className="split-inspector">
                <InspectorBlock title="Raw request" value={requestRaw} />
                <InspectorBlock title="Raw response" value={responseRaw} />
              </div>
            )}
          </div>
        </section>
      </main>

      <section className="log-strip" title={logLines.slice(-8).join("\n")}>
        <span className="log-label">LOG</span>
        <span>{notice}</span>
      </section>

      <footer className="status-bar">
        <span>{state?.running ? `Proxy: 127.0.0.1:${state.settings.proxyPort}` : "Proxy stopped"}</span>
        <span>Monitor: {state ? `127.0.0.1:${state.settings.monitorPort}` : "—"}</span>
        <span>CA: {state?.caPath || "—"}</span>
        <span className="status-grow" />
        <span>{source === "live" ? "Live capture" : "Offline review"}</span>
      </footer>

      {showSettings && (
        <div className="modal-backdrop" role="presentation" onMouseDown={() => setShowSettings(false)}>
          <section className="settings-modal" role="dialog" aria-modal="true" onMouseDown={(event) => event.stopPropagation()}>
            <header>
              <div>
                <strong>TLSDebug Settings</strong>
                <span>Proxy process and storage</span>
              </div>
              <button className="close-button" onClick={() => setShowSettings(false)}>×</button>
            </header>
            <label>
              Proxy port
              <input
                type="number"
                value={draftSettings.proxyPort}
                onChange={(event) => setDraftSettings({ ...draftSettings, proxyPort: Number(event.target.value) })}
              />
            </label>
            <label>
              Monitor port
              <input
                type="number"
                value={draftSettings.monitorPort}
                onChange={(event) => setDraftSettings({ ...draftSettings, monitorPort: Number(event.target.value) })}
              />
            </label>
            <label>
              Data directory
              <input
                value={draftSettings.dataDir}
                onChange={(event) => setDraftSettings({ ...draftSettings, dataDir: event.target.value })}
              />
            </label>
            <label>
              Proxy binary override
              <input
                value={draftSettings.proxyBinary}
                onChange={(event) => setDraftSettings({ ...draftSettings, proxyBinary: event.target.value })}
                placeholder="Leave empty to auto-detect tlsproxy"
              />
            </label>
            <p className="settings-hint">Settings can only be changed while capture is stopped.</p>
            <footer>
              <button onClick={() => setShowSettings(false)}>Cancel</button>
              <button className="primary" disabled={busy || state?.running} onClick={saveSettings}>Save settings</button>
            </footer>
          </section>
        </div>
      )}
    </div>
  );
}

function InspectorBlock({ title, value }: { title: string; value: string }) {
  return (
    <section className="inspector-block">
      <header>{title}</header>
      <CodeView value={value} />
    </section>
  );
}

function CodeView({ value }: { value: string }) {
  return <pre className="code-view">{value}</pre>;
}

export default App;
