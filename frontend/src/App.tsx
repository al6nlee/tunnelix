import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import {
  FlaskConical,
  LoaderCircle,
  ArrowDownToLine,
  ArrowRight,
  ArrowUpFromLine,
  Check,
  ChevronRight,
  CircleHelp,
  Copy,
  KeyRound,
  Network,
  Pencil,
  Plus,
  Power,
  Search,
  Settings2,
  Sun,
  Moon,
  Trash2,
  Terminal,
  X,
} from "lucide-react";
import { UpdatePanel } from "./components/updates";
import { CredentialFields, PasswordInput } from "./components/credentials";
import { Button } from "./components/ui/button";
import { Dialog } from "./components/ui/dialog";
import {
  call,
  Events,
  emptyUpdate,
  type UpdateStatus,
  desktopCall,
  desktop,
  emptySnapshot,
  defaultNetwork,
  type NetworkOptions,
  subscribe,
  type Challenge,
  type CredentialInfo,
  type CredentialUpdate,
  type Mapping,
  type Settings,
  type Snapshot,
  type Status,
  type Tunnel,
} from "./api";
const isActive = (s: string) =>
  ["connected", "connecting", "reconnecting"].includes(s);
const mapping = (): Mapping => ({
  type: "local",
  bind: "127.0.0.1",
  port: 5432,
  target: "127.0.0.1",
  targetPort: 5432,
});
const blank = (): Tunnel => ({
  network: { ...defaultNetwork },
  id: crypto.randomUUID(),
  name: "",
  host: "",
  port: 0,
  user: "",
  auth: "agent",
  identity: "",
  jump: "",
  autoConnect: false,
  reconnect: true,
  mappings: [mapping()],
});
function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
function address(host: string, port: number) {
  return `${host.includes(":") ? `[${host}]` : host}:${port}`;
}

export default function App() {
  const { t, i18n } = useTranslation();
  const [data, setData] = useState<Snapshot>(emptySnapshot),
    [query, setQuery] = useState(""),
    [filter, setFilter] = useState("all"),
    [selected, setSelected] = useState<string | null>(null),
    [editor, setEditor] = useState<Tunnel | null>(null),
    [settings, setSettings] = useState(false),
    [deleting, setDeleting] = useState<Tunnel | null>(null),
    [message, setMessage] = useState(""),
    [failure, setFailure] = useState(false),
    [busy, setBusy] = useState(false);
  const [update, setUpdate] = useState(emptyUpdate);
  const [themePreview, setThemePreview] = useState<Settings["theme"] | null>(
    null,
  );
  useLayoutEffect(() => {
    document.documentElement.dataset.theme =
      themePreview ?? data.config.settings.theme;
  }, [themePreview, data.config.settings.theme]);
  useEffect(() => {
    if (!desktop) return;
    let stopped = false;
    let timer = 0;
    const refreshUpdate = () => {
      void desktopCall<UpdateStatus>("UpdateStatus")
        .then((next) => {
          if (!stopped) {
            setUpdate(next);
            stopped = true;
            window.clearInterval(timer);
          }
        })
        .catch(() => {});
    };
    // Bindings may become available a little after the first React render.
    // Keep trying until the initial status is received, otherwise the panel
    // would remain on an empty version forever after a transient startup race.
    timer = window.setInterval(refreshUpdate, 100);
    refreshUpdate();
    const off = Events.On("tunnelix:update", () => {
      void desktopCall<UpdateStatus>("UpdateStatus")
        .then(setUpdate)
        .catch(() => {});
    });
    const settingsOff = Events.On("tunnelix:settings", () => setSettings(true));
    const detailOff = Events.On("tunnelix:detail", (event) => {
      const id = typeof event.data === "string" ? event.data : event.data?.[0];
      if (typeof id === "string") {
        setQuery("");
        setFilter("all");
        setSelected(id);
      }
    });
    const showAllOff = Events.On("tunnelix:show-all", () => {
      setQuery("");
      setFilter("all");
    });
    return () => {
      stopped = true;
      window.clearInterval(timer);
      off();
      settingsOff();
      detailOff();
      showAllOff();
    };
  }, []);
  const searchInput = useRef<HTMLInputElement>(null);
  const input = useRef<HTMLInputElement>(null),
    revision = useRef(0);
  const errorText = useCallback(
    (e: unknown) => {
      const value =
        e instanceof Error
          ? e.message
          : typeof e === "string"
            ? e
            : JSON.stringify(e);
      const code = value?.match(/[A-Z][A-Z_]{3,}/)?.[0];
      return code
        ? t(`errors.${code}`, { defaultValue: t("unknownError") })
        : t("unknownError");
    },
    [t],
  );
  const report = useCallback(
    (e: unknown) => {
      setMessage(errorText(e));
      setFailure(true);
    },
    [errorText],
  );
  const refresh = useCallback(async () => {
    if (!desktop) return;
    const rev = ++revision.current;
    try {
      const next = await call<Snapshot>("Snapshot");
      if (rev === revision.current) {
        setData(next);
      }
    } catch (e) {
      report(e);
    }
  }, [report]);
  useEffect(() => {
    void refresh();
    if (!desktop) return;
    const off = subscribe(() => void refresh());
    return () => {
      off();
      revision.current++;
    };
  }, [refresh]);
  useEffect(() => {
    const lang = data.config.settings.language;
    void i18n.changeLanguage(
      lang === "system"
        ? navigator.language.toLowerCase().startsWith("zh")
          ? "zh-CN"
          : "en"
        : lang,
    );
  }, [data.config.settings.language, i18n]);
  useEffect(() => {
    if (!desktop) return;
    // The Wails runtime can become callable shortly after React mounts. Retry
    // briefly so headless/slow hosted runners do not lose the one ready call.
    let stopped = false;
    let timer = 0;
    const notify = () => {
      void desktopCall("FrontendReady")
        .then(() => {
          stopped = true;
          window.clearInterval(timer);
        })
        .catch(() => {});
    };
    timer = window.setInterval(() => {
      if (!stopped) notify();
    }, 100);
    notify();
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, []);
  useEffect(() => {
    document.documentElement.lang = i18n.language;
  }, [i18n.language]);
  useEffect(() => {
    if (!message) return;
    const timer = setTimeout(() => setMessage(""), 6500);
    return () => clearTimeout(timer);
  }, [message]);
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (
        !(e.metaKey || e.ctrlKey) ||
        e.altKey ||
        document.querySelector('[role="dialog"]')
      )
        return;
      if (e.key.toLowerCase() === "n") {
        e.preventDefault();
        setEditor(blank());
      }
      if (e.key.toLowerCase() === "f") {
        e.preventDefault();
        searchInput.current?.focus();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, []);
  async function run(fn: () => Promise<unknown>, success?: string) {
    setBusy(true);
    try {
      await fn();
      await refresh();
      if (success) {
        setMessage(success);
        setFailure(false);
      }
      return true;
    } catch (e) {
      report(e);
      return false;
    } finally {
      setBusy(false);
    }
  }
  const state = (id: string) =>
    data.statuses.find((s) => s.id === id)?.state ?? "disconnected";
  const visible = data.config.tunnels.filter(
    (tunnel) =>
      `${tunnel.name} ${tunnel.host}`
        .toLowerCase()
        .includes(query.toLowerCase()) &&
      (filter === "all" ||
        (filter === "active"
          ? isActive(state(tunnel.id))
          : !isActive(state(tunnel.id)))),
  );
  const detail = visible.find((t) => t.id === selected),
    status = data.statuses.find((s) => s.id === selected);
  const challenge = data.challenges[0];
  const toggle = (tunnel: Tunnel) =>
    run(() =>
      call(isActive(state(tunnel.id)) ? "Disconnect" : "Connect", tunnel.id),
    );
  return (
    <div className="app-shell">
      <header className="brand-header">
        <div className="app-brand">
          <img src="/icon.png" alt="" />
          <div>
            <h1>Tunnelix</h1>
            <p>{t("brandDescriptor")}</p>
          </div>
        </div>
        <div className="brand-message">
          <h2>{t("brandPromise")}</h2>
          <p>{t("brandSupporting")}</p>
        </div>
        <div className="brand-orbit" aria-hidden="true">
          <i />
          <i />
          <i />
          <span />
        </div>
      </header>
      <div className="app-toolbar">
        <div className="search">
          <Search size={15} />
          <input
            ref={searchInput}
            aria-label={t("search")}
            placeholder={t("search")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          {query && (
            <button
              aria-label={t("clearSearch")}
              title={t("clearSearch")}
              onClick={() => {
                setQuery("");
                searchInput.current?.focus();
              }}
            >
              <X size={14} />
            </button>
          )}
        </div>
        <div
          className="status-filters"
          role="group"
          aria-label={t("filterStatus")}
        >
          {["all", "active", "offline"].map((f) => (
            <button
              key={f}
              aria-pressed={filter === f}
              onClick={() => setFilter(f)}
            >
              {t(f)}
            </button>
          ))}
        </div>
        <Button size="sm" onClick={() => setEditor(blank())}>
          <Plus size={15} />
          {t("newTunnel")}
        </Button>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t("settings")}
          title={t("settings")}
          onClick={() => setSettings(true)}
        >
          <Settings2 size={18} />
        </Button>
      </div>
      {!desktop && (
        <div className="app-notice">
          <CircleHelp size={14} />
          {t("preview")}
        </div>
      )}
      {desktop && !data.sshAvailable && (
        <button
          className="app-notice warning"
          onClick={() => setSettings(true)}
        >
          <CircleHelp size={14} />
          {t("sshMissing")}
        </button>
      )}
      <main className="workspace">
        <section className="connections" aria-label={t("tunnels")}>
          <div className="list-heading">
            <h2>{t("tunnels")}</h2>
            <span>{t("visibleCount", { count: visible.length })}</span>
          </div>
          <div className="connection-scroll">
            {data.config.tunnels.length === 0 ? (
              <div className="list-empty">
                <Network size={30} />
                <h2>{t("emptyTitle")}</h2>
                <p>{t("emptyText")}</p>
                <Button size="sm" onClick={() => setEditor(blank())}>
                  <Plus size={15} />
                  {t("addFirst")}
                </Button>
              </div>
            ) : visible.length === 0 ? (
              <div className="list-empty">
                <Search size={26} />
                <p>{t("noResults")}</p>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setQuery("");
                    setFilter("all");
                  }}
                >
                  {t("clearFilters")}
                </Button>
              </div>
            ) : (
              <div className="connection-list" role="list">
                {visible.map((tunnel, index) => {
                  const s = state(tunnel.id),
                    m = tunnel.mappings[0];
                  const summary = m
                    ? `${t(m.type)} · ${address(m.bind, m.port)} → ${m.type === "dynamic" ? "SOCKS5" : address(m.target, m.targetPort)}`
                    : "";
                  return (
                    <article
                      key={tunnel.id}
                      id={`tunnel-row-${tunnel.id}`}
                      role="listitem"
                      className={`connection-row ${detail?.id === tunnel.id ? "selected" : ""}`}
                    >
                      <button
                        className="connection-select"
                        aria-label={t("selectTunnel", { name: tunnel.name })}
                        aria-pressed={detail?.id === tunnel.id}
                        onClick={() => setSelected(tunnel.id)}
                        onKeyDown={(e) => {
                          if (e.key !== "ArrowDown" && e.key !== "ArrowUp")
                            return;
                          e.preventDefault();
                          const next =
                            visible[index + (e.key === "ArrowDown" ? 1 : -1)];
                          if (next) {
                            setSelected(next.id);
                            document
                              .getElementById(`tunnel-row-${next.id}`)
                              ?.querySelector("button")
                              ?.focus();
                          }
                        }}
                      >
                        <span className={`connection-dot ${s}`} />
                        <span className="connection-copy">
                          <span className="connection-title">
                            <strong title={tunnel.name}>{tunnel.name}</strong>
                            <span className={`status-badge ${s}`}>{t(s)}</span>
                          </span>
                          <span
                            className="connection-host"
                            title={`${tunnel.user ? `${tunnel.user}@` : ""}${tunnel.port ? address(tunnel.host, tunnel.port) : tunnel.host}`}
                          >
                            {tunnel.user ? `${tunnel.user}@` : ""}
                            {tunnel.port
                              ? address(tunnel.host, tunnel.port)
                              : tunnel.host}
                          </span>
                          <span className="connection-forward" title={summary}>
                            <span>{summary}</span>
                            {tunnel.mappings.length > 1 && (
                              <b>+{tunnel.mappings.length - 1}</b>
                            )}
                          </span>
                        </span>
                      </button>
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy}
                        aria-label={`${t(isActive(s) ? "disconnect" : "connect")} ${tunnel.name}`}
                        onClick={() => void toggle(tunnel)}
                      >
                        <Power size={13} />
                        {t(isActive(s) ? "disconnect" : "connect")}
                      </Button>
                    </article>
                  );
                })}
              </div>
            )}
          </div>
        </section>
        <aside className="inspector" aria-label={t("details")}>
          {detail ? (
            <>
              <div className="inspector-heading">
                <h2 title={detail.name}>{detail.name}</h2>
                <Button
                  variant="ghost"
                  size="icon"
                  title={t("closeDetails")}
                  aria-label={t("closeDetails")}
                  onClick={() => setSelected(null)}
                >
                  <X size={16} />
                </Button>
              </div>
              <div className="inspector-scroll" key={detail.id}>
                <div className="detail-heading">
                  <span className={`status-badge ${state(detail.id)}`}>
                    {t(state(detail.id))}
                  </span>
                  <code>
                    {detail.user ? `${detail.user}@` : ""}
                    {detail.port
                      ? address(detail.host, detail.port)
                      : detail.host}
                  </code>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy}
                    onClick={() => void toggle(detail)}
                  >
                    {t(isActive(state(detail.id)) ? "disconnect" : "connect")}
                  </Button>
                </div>
                {status?.code && (
                  <div className="notice warning">
                    {t(`errors.${status.code}`, { defaultValue: status.code })}
                  </div>
                )}
                <h3 className="section-title">{t("mappings")}</h3>
                {detail.mappings.map((m, i) => (
                  <div className="detail-mapping" key={i}>
                    <span className="type-pill">{t(m.type)}</span>
                    <code>{address(m.bind, m.port)}</code>
                    <ArrowRight size={15} />
                    <code>
                      {m.type === "dynamic"
                        ? "SOCKS5"
                        : address(m.target, m.targetPort)}
                    </code>
                    {m.type !== "remote" && (
                      <Button
                        variant="ghost"
                        size="icon"
                        title={t("copy")}
                        onClick={() =>
                          void run(
                            () =>
                              navigator.clipboard.writeText(
                                `${m.type === "dynamic" ? "socks5h://" : ""}${address(m.bind, m.port)}`,
                              ),
                            t("copied"),
                          )
                        }
                      >
                        <Copy size={14} />
                      </Button>
                    )}
                  </div>
                ))}
                {status?.state === "connected" && (
                  <p className="hint">{t("forwardingNote")}</p>
                )}
                <CommandPreview
                  tunnel={detail}
                  actual={
                    isActive(state(detail.id)) ? status?.command : undefined
                  }
                  report={report}
                />
                <h3 className="section-title">{t("diagnostics")}</h3>
                <div className="event-list">
                  {status?.logs?.length ? (
                    [...status.logs].reverse().map((log, i) => (
                      <div key={i}>
                        <time>
                          {new Date(log.time).toLocaleTimeString(i18n.language)}
                        </time>
                        <span>
                          {t(log.code, {
                            defaultValue: t(`errors.${log.code}`, {
                              defaultValue: log.code,
                            }),
                          })}
                        </span>
                      </div>
                    ))
                  ) : (
                    <p className="muted">{t("noEvents")}</p>
                  )}
                </div>
              </div>
              <div className="inspector-actions">
                <Button
                  variant="outline"
                  onClick={() => {
                    setEditor({
                      ...structuredClone(detail),
                      id: crypto.randomUUID(),
                      name: t("copyName", { name: detail.name }),
                      autoConnect: false,
                    });
                  }}
                >
                  <Copy size={14} />
                  {t("duplicateTunnel")}
                </Button>
                <Button
                  variant="destructive"
                  disabled={isActive(state(detail.id))}
                  onClick={() => {
                    setDeleting(detail);
                  }}
                >
                  <Trash2 size={14} />
                  {t("delete")}
                </Button>
                <Button
                  variant="outline"
                  disabled={isActive(state(detail.id))}
                  onClick={() => {
                    setEditor(structuredClone(detail));
                  }}
                >
                  <Pencil size={14} />
                  {t("edit")}
                </Button>
              </div>
            </>
          ) : (
            <div className="inspector-empty">
              <Network size={28} />
              <h2>{t("selectConnection")}</h2>
              <p>{t("selectConnectionHint")}</p>
            </div>
          )}
        </aside>
      </main>
      <footer className="statusbar" title={t("shortcuts")}>
        <span>
          {t("connectionSummary", {
            total: data.config.tunnels.length,
            connected: data.statuses.filter((s) => s.state === "connected")
              .length,
          })}
        </span>
        <span className="engine-status">
          <span
            className={`connection-dot ${data.sshAvailable ? "connected" : "disconnected"}`}
          />
          {data.sshAvailable ? "OpenSSH" : t("sshUnavailable")}
        </span>
        <button
          onClick={() => setSettings(true)}
          aria-label={t("updates.title")}
        >
          {update.state === "ready"
            ? t("updates.restart")
            : update.current
              ? `v${update.current}`
              : t("settings")}
        </button>
      </footer>
      {editor && (
        <TunnelEditor
          key={editor.id}
          tests={data.tests}
          credentialInfo={data.credentials[editor.id]}
          tunnel={editor}
          existing={data.config.tunnels.some((t) => t.id === editor.id)}
          onClose={() => setEditor(null)}
          onSave={async (value, credential) => {
            await call("SaveTunnelWithCredential", value, credential);
            await refresh();
            setEditor(null);
            setQuery("");
            setFilter("all");
            setSelected(value.id);
            setMessage(t("saved"));
            setFailure(false);
          }}
          report={report}
        />
      )}
      {settings && (
        <SettingsDialog
          settings={data.config.settings}
          onThemePreview={setThemePreview}
          update={update}
          active={
            data.statuses.some((s) => isActive(s.state)) ||
            data.tests.some((s) => s.state === "testing" || isActive(s.state))
          }
          onClose={() => {
            setThemePreview(null);
            setSettings(false);
          }}
          onSave={async (s) => {
            if (await run(() => call("SaveSettings", s), t("saved"))) {
              setThemePreview(null);
              setSettings(false);
            }
          }}
          onImport={() => input.current?.click()}
          onExport={() => void run(() => desktopCall("ExportFile"))}
          report={report}
        />
      )}
      <input
        ref={input}
        type="file"
        accept=".json,application/json"
        hidden
        onChange={async (e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (!file) return;
          if (file.size > 2 * 1024 * 1024) {
            report(new Error("CONFIG_TOO_LARGE"));
            return;
          }
          await run(
            async () => call("ImportConfig", await file.text()),
            t("imported"),
          );
        }}
      />
      <Dialog
        open={!!deleting}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title={t("deleteTitle")}
        description={t("deleteHint")}
      >
        <p>
          <strong>{deleting?.name}</strong>
        </p>
        <div className="dialog-footer">
          <Button variant="outline" onClick={() => setDeleting(null)}>
            {t("cancel")}
          </Button>
          <Button
            variant="destructive"
            disabled={busy}
            onClick={() => {
              if (deleting)
                void run(() => call("DeleteTunnel", deleting.id)).then((ok) => {
                  if (ok) {
                    setDeleting(null);
                    setSelected(null);
                  }
                });
            }}
          >
            {t("confirmDelete")}
          </Button>
        </div>
      </Dialog>
      {challenge && (
        <AuthDialog
          key={challenge.id}
          challenge={challenge}
          respond={async (value, accepted) => {
            await run(() => call("Respond", challenge.id, value, accepted));
          }}
        />
      )}
      {message && (
        <div role="status" className={`toast ${failure ? "toast-error" : ""}`}>
          {failure ? <CircleHelp size={17} /> : <Check size={17} />}
          <span>{message}</span>
          <button onClick={() => setMessage("")} aria-label={t("close")}>
            <X size={15} />
          </button>
        </div>
      )}
    </div>
  );
}

function TunnelEditor({
  tests,
  credentialInfo,
  tunnel,
  existing,
  onClose,
  onSave,
  report,
}: {
  tests: Status[];
  credentialInfo?: CredentialInfo;
  tunnel: Tunnel;
  existing: boolean;
  onClose: () => void;
  onSave: (t: Tunnel, credential: CredentialUpdate) => Promise<void>;
  report: (e: unknown) => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(tunnel),
    [saving, setSaving] = useState(false),
    [aliases, setAliases] = useState<string[]>([]);
  const [credential, setCredential] = useState<CredentialUpdate>({
    mode: credentialInfo?.saved ? "keep" : "none",
    value: "",
  });
  const [saveError, setSaveError] = useState("");
  const [discard, setDiscard] = useState(false);
  const identityChanged = [
    "host",
    "port",
    "user",
    "auth",
    "identity",
    "jump",
  ].some((k) => draft[k as keyof Tunnel] !== tunnel[k as keyof Tunnel]);
  const dirty =
    JSON.stringify(draft) !== JSON.stringify(tunnel) ||
    credential.value !== "" ||
    credential.mode !== (credentialInfo?.saved ? "keep" : "none");
  const credentialInput = (): CredentialUpdate =>
    draft.auth === "password" ? credential : { mode: "none", value: "" };
  const describeError = (e: unknown) => {
    const text = e instanceof Error ? e.message : JSON.stringify(e);
    const code = text?.match(/[A-Z][A-Z_]{3,}/)?.[0];
    return code
      ? t(`errors.${code}`, { defaultValue: t("unknownError") })
      : t("unknownError");
  };
  useEffect(() => {
    if (desktop) void call<string[]>("Aliases").then(setAliases).catch(report);
  }, [report]);
  const [testId, setTestId] = useState<string | null>(null),
    [startingTest, setStartingTest] = useState(false),
    [testError, setTestError] = useState("");
  const testIdRef = useRef<string | null>(null),
    mounted = useRef(true),
    formRef = useRef<HTMLFormElement>(null);
  const test = tests.find((item) => item.id === testId);
  const testing =
    startingTest || (!!testId && (!test || test.state === "testing"));
  const clearTest = () => {
    const id = testIdRef.current;
    testIdRef.current = null;
    setTestId(null);
    setTestError("");
    if (id) void call("ClearTest", id).catch(report);
  };
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const id = testIdRef.current;
      testIdRef.current = null;
      if (id) void call("ClearTest", id).catch(() => {});
    };
  }, []);
  async function startTest() {
    if (testing || !formRef.current?.reportValidity()) return;
    clearTest();
    setStartingTest(true);
    try {
      const id = await call<string>("TestDraft", draft, credentialInput());
      if (!mounted.current) {
        await call("ClearTest", id);
        return;
      }
      testIdRef.current = id;
      setTestId(id);
    } catch (e) {
      if (mounted.current) {
        const message = e instanceof Error ? e.message : JSON.stringify(e);
        const code = message.match(/[A-Z][A-Z_]{3,}/)?.[0];
        setTestError(
          code
            ? t(`errors.${code}`, { defaultValue: t("unknownError") })
            : t("unknownError"),
        );
      }
    } finally {
      if (mounted.current) setStartingTest(false);
    }
  }
  const set = <K extends keyof Tunnel>(key: K, value: Tunnel[K]) => {
    clearTest();
    setSaveError("");
    setDraft((d) => ({ ...d, [key]: value }));
  };
  const network = draft.network ?? defaultNetwork;
  const setNetwork = (key: keyof NetworkOptions, value: number) =>
    set("network", { ...network, [key]: value });
  const setMap = (i: number, patch: Partial<Mapping>) =>
    set(
      "mappings",
      draft.mappings.map((m, n) => (n === i ? { ...m, ...patch } : m)),
    );
  const requestClose = () => {
    if (saving) return;
    if (dirty || testing) setDiscard(true);
    else onClose();
  };
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (
        (e.metaKey || e.ctrlKey) &&
        e.key.toLowerCase() === "s" &&
        document.querySelectorAll('[role="dialog"]').length === 1
      ) {
        e.preventDefault();
        if (!saving && !testing) formRef.current?.requestSubmit();
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [saving, testing]);
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) requestClose();
      }}
      title={t(existing ? "editTunnel" : "newTunnel")}
      description={t("tunnelFormHint")}
      editor
      wide
    >
      <form
        ref={formRef}
        onSubmit={async (e) => {
          e.preventDefault();
          if (testing) return;
          setSaving(true);
          try {
            setSaveError("");
            await onSave(draft, credentialInput());
          } catch (e) {
            setSaveError(describeError(e));
          } finally {
            setSaving(false);
          }
        }}
      >
        <div className="form-scroll editor-scroll">
          <fieldset className="editor-fields" disabled={testing || saving}>
            <Field label={t("name")}>
              <input
                autoFocus
                required
                maxLength={200}
                placeholder={t("namePlaceholder")}
                value={draft.name}
                onChange={(e) => set("name", e.target.value)}
              />
            </Field>
            <div className="form-grid">
              <Field label={t("host")}>
                <input
                  required
                  list="ssh-aliases"
                  placeholder="server.example.com"
                  value={draft.host}
                  onChange={(e) => set("host", e.target.value.trim())}
                />
                <datalist id="ssh-aliases">
                  {aliases.map((a) => (
                    <option key={a} value={a} />
                  ))}
                </datalist>
              </Field>
              <Field label={t("port")} hint={t("portHint")}>
                <input
                  type="number"
                  required
                  min={0}
                  max={65535}
                  value={draft.port}
                  onChange={(e) => set("port", Number(e.target.value))}
                />
              </Field>
              <Field label={t("user")}>
                <input
                  placeholder={t("userHint")}
                  value={draft.user}
                  onChange={(e) => set("user", e.target.value.trim())}
                />
              </Field>
              <Field label={t("authentication")}>
                <select
                  value={draft.auth}
                  onChange={(e) =>
                    set("auth", e.target.value as Tunnel["auth"])
                  }
                >
                  {["agent", "key", "password"].map((a) => (
                    <option key={a} value={a}>
                      {t(a)}
                    </option>
                  ))}
                </select>
              </Field>
            </div>
            {draft.auth === "key" && (
              <Field label={t("identity")} hint={t("keyHint")}>
                <div className="input-action">
                  <input
                    required
                    placeholder="~/.ssh/id_ed25519"
                    value={draft.identity}
                    onChange={(e) => set("identity", e.target.value)}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() =>
                      void desktopCall<string>("ChooseFile")
                        .then((p) => {
                          if (p) set("identity", p);
                        })
                        .catch(report)
                    }
                  >
                    {t("browse")}
                  </Button>
                </div>
              </Field>
            )}
            {draft.auth === "password" && (
              <CredentialFields
                info={identityChanged ? undefined : credentialInfo}
                identityChanged={identityChanged && !!credentialInfo?.saved}
                update={credential}
                onChange={(value) => {
                  clearTest();
                  setSaveError("");
                  setCredential(value);
                }}
              />
            )}
            <Field
              label={t("jump")}
              hint={draft.jump ? t("jumpAuthHint") : undefined}
            >
              <input
                placeholder={t("jumpHint")}
                value={draft.jump}
                onChange={(e) => set("jump", e.target.value.trim())}
              />
            </Field>
            <div className="section-heading">
              <h3>{t("forwarding")}</h3>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => set("mappings", [...draft.mappings, mapping()])}
              >
                <Plus size={14} />
                {t("addMapping")}
              </Button>
            </div>
            {draft.mappings.map((m, i) => (
              <div className="mapping-editor" key={i}>
                <div className="mapping-editor-head">
                  <select
                    aria-label={t("forwarding")}
                    value={m.type}
                    onChange={(e) =>
                      setMap(i, { type: e.target.value as Mapping["type"] })
                    }
                  >
                    {["local", "remote", "dynamic"].map((type) => (
                      <option key={type} value={type}>
                        {t(type)}
                      </option>
                    ))}
                  </select>
                  <span>#{String(i + 1).padStart(2, "0")}</span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    title={t("removeMapping")}
                    disabled={draft.mappings.length === 1}
                    onClick={() =>
                      set(
                        "mappings",
                        draft.mappings.filter((_, n) => n !== i),
                      )
                    }
                  >
                    <Trash2 size={14} />
                  </Button>
                </div>
                <div className="mapping-fields">
                  <Field label={t("bind")}>
                    <input
                      required
                      value={m.bind}
                      onChange={(e) =>
                        setMap(i, { bind: e.target.value.trim() })
                      }
                    />
                  </Field>
                  <Field label={t("listenPort")}>
                    <input
                      required
                      type="number"
                      min={1}
                      max={65535}
                      value={m.port}
                      onChange={(e) =>
                        setMap(i, { port: Number(e.target.value) })
                      }
                    />
                  </Field>
                  {m.type !== "dynamic" && (
                    <>
                      <Field label={t("target")}>
                        <input
                          required
                          value={m.target}
                          onChange={(e) =>
                            setMap(i, { target: e.target.value.trim() })
                          }
                        />
                      </Field>
                      <Field label={t("targetPort")}>
                        <input
                          required
                          type="number"
                          min={1}
                          max={65535}
                          value={m.targetPort}
                          onChange={(e) =>
                            setMap(i, { targetPort: Number(e.target.value) })
                          }
                        />
                      </Field>
                    </>
                  )}
                </div>
                <p className="hint">{t(`${m.type}MappingHint`)}</p>
              </div>
            ))}
            <details
              className="network-options"
              onInvalidCapture={(e) => {
                e.currentTarget.open = true;
              }}
            >
              <summary>{t("advancedSettings")}</summary>
              <p className="hint">{t("networkOptionsHint")}</p>
              <div className="form-grid">
                <Field
                  label={t("connectTimeout")}
                  hint={t("connectTimeoutHint")}
                >
                  <input
                    type="number"
                    required
                    min={5}
                    max={120}
                    step={1}
                    value={network.connectTimeout || ""}
                    onChange={(e) =>
                      setNetwork("connectTimeout", Number(e.target.value))
                    }
                  />
                </Field>
                <Field
                  label={t("serverAliveInterval")}
                  hint={t("serverAliveIntervalHint")}
                >
                  <input
                    type="number"
                    required
                    min={5}
                    max={300}
                    step={1}
                    value={network.serverAliveInterval || ""}
                    onChange={(e) =>
                      setNetwork("serverAliveInterval", Number(e.target.value))
                    }
                  />
                </Field>
                <Field
                  label={t("serverAliveCountMax")}
                  hint={t("serverAliveCountMaxHint")}
                >
                  <input
                    type="number"
                    required
                    min={1}
                    max={10}
                    step={1}
                    value={network.serverAliveCountMax || ""}
                    onChange={(e) =>
                      setNetwork("serverAliveCountMax", Number(e.target.value))
                    }
                  />
                </Field>
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => set("network", { ...defaultNetwork })}
              >
                {t("restoreNetworkDefaults")}
              </Button>
            </details>
            <CommandPreview tunnel={draft} report={report} />
            <h3 className="section-title">{t("behavior")}</h3>
            <label className="check-row">
              <input
                type="checkbox"
                checked={draft.reconnect}
                onChange={(e) => set("reconnect", e.target.checked)}
              />
              {t("reconnect")}
            </label>
            <label className="check-row">
              <input
                type="checkbox"
                checked={draft.autoConnect}
                onChange={(e) => set("autoConnect", e.target.checked)}
              />
              {t("autoConnect")}
            </label>
          </fieldset>
        </div>
        <div className="test-result" role="status" aria-live="polite">
          {testing ? (
            <p>
              <LoaderCircle size={15} className="spin" />
              {t("testing")}
            </p>
          ) : testError ? (
            <p className="test-failed">{testError}</p>
          ) : test?.state === "testPassed" ? (
            <p className="test-passed">
              <Check size={15} />
              {t("testPassed")}
            </p>
          ) : test?.state === "error" ? (
            <p className="test-failed">
              {t(`errors.${test.code}`, { defaultValue: t("unknownError") })}
            </p>
          ) : test?.state === "testCancelled" ? (
            <p>{t("testCancelled")}</p>
          ) : null}
          <small>{t("testHint")}</small>
        </div>
        {saveError && (
          <div role="alert" className="notice warning">
            {saveError}
          </div>
        )}
        <div className="dialog-footer editor-footer">
          <Button
            type="button"
            variant="outline"
            disabled={saving || startingTest}
            onClick={() => {
              if (testing && testId)
                void call("CancelTest", testId).catch(report);
              else void startTest();
            }}
          >
            {testing ? <X size={15} /> : <FlaskConical size={15} />}
            {t(testing ? "cancelTest" : "testConnection")}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={requestClose}
            disabled={saving}
          >
            {t("cancel")}
          </Button>
          <Button type="submit" disabled={saving || testing}>
            {saving && <LoaderCircle size={15} className="spin" />}
            {t(existing ? "save" : "create")}
          </Button>
        </div>
      </form>
      <DiscardDialog
        open={discard}
        onKeep={() => setDiscard(false)}
        onDiscard={onClose}
      />
    </Dialog>
  );
}
function SettingsDialog({
  settings,
  onThemePreview,
  update,
  active,
  onClose,
  onSave,
  onImport,
  onExport,
  report,
}: {
  settings: Settings;
  onThemePreview: (theme: Settings["theme"]) => void;
  update: UpdateStatus;
  active: boolean;
  onClose: () => void;
  onSave: (s: Settings) => Promise<void>;
  onImport: () => void;
  onExport: () => void;
  report: (e: unknown) => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState(settings),
    [saving, setSaving] = useState(false);
  const [discard, setDiscard] = useState(false);
  const requestClose = () => {
    if (saving) return;
    if (JSON.stringify(draft) !== JSON.stringify(settings)) setDiscard(true);
    else onClose();
  };
  return (
    <Dialog
      open
      wide
      editor
      className="dialog-settings"
      onOpenChange={(open) => {
        if (!open) requestClose();
      }}
      title={t("settings")}
      description={t("settingsIntro")}
    >
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setSaving(true);
          try {
            await onSave(draft);
          } finally {
            setSaving(false);
          }
        }}
      >
        <div className="settings-scroll">
          <section className="settings-section">
            <fieldset
              className="theme-picker settings-appearance"
              disabled={saving}
            >
              <legend className="sr-only">{t("theme")}</legend>
              <div className="settings-label">
                <h3>{t("theme")}</h3>
                <p>{t("themeHint")}</p>
              </div>
              <div className="theme-options">
                {(["light", "dark"] as const).map((theme) => (
                  <label
                    key={theme}
                    className={`theme-option ${draft.theme === theme ? "chosen" : ""}`}
                  >
                    <input
                      type="radio"
                      name="theme"
                      value={theme}
                      checked={draft.theme === theme}
                      onChange={() => {
                        setDraft({ ...draft, theme });
                        onThemePreview(theme);
                      }}
                    />
                    <span
                      className={`theme-sample ${theme}`}
                      aria-hidden="true"
                    >
                      <i />
                      <span>
                        <b />
                        <b />
                        <b />
                      </span>
                    </span>
                    <span className="theme-label">
                      {theme === "light" ? (
                        <Sun size={14} />
                      ) : (
                        <Moon size={14} />
                      )}
                      {t(`theme_${theme}`)}
                      {draft.theme === theme && <Check size={14} />}
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>
            <div className="settings-row">
              <span className="settings-row-title" id="settings-language-label">
                {t("language")}
              </span>
              <div
                className="segmented-choice"
                role="group"
                aria-labelledby="settings-language-label"
              >
                {(["system", "zh-CN", "en"] as const).map((language) => (
                  <label
                    key={language}
                    className={draft.language === language ? "chosen" : ""}
                  >
                    <input
                      type="radio"
                      name="language"
                      value={language}
                      disabled={saving}
                      checked={draft.language === language}
                      onChange={() => setDraft({ ...draft, language })}
                    />
                    {language === "system"
                      ? t("system")
                      : language === "en"
                        ? "English"
                        : "简体中文"}
                  </label>
                ))}
              </div>
            </div>
          </section>
          <section className="settings-section settings-engine">
            <div className="settings-label">
              <h3>
                <Terminal size={15} />
                {t("sshExecutable")}
              </h3>
              <p id="ssh-path-help">{t("sshPathHint")}</p>
            </div>
            <div className="settings-path">
              <input
                aria-label={t("sshExecutable")}
                aria-describedby="ssh-path-help"
                value={draft.sshPath}
                placeholder={t("sshAutoDetect")}
                disabled={saving}
                onChange={(e) =>
                  setDraft({ ...draft, sshPath: e.target.value })
                }
              />
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={saving}
                onClick={() =>
                  void desktopCall<string>("ChooseFile")
                    .then((p) => {
                      if (p)
                        setDraft((current) => ({ ...current, sshPath: p }));
                    })
                    .catch(report)
                }
              >
                {t("browse")}
              </Button>
            </div>
          </section>
          <UpdatePanel
            status={update}
            active={active}
            autoUpdate={draft.autoUpdate}
            disabled={saving}
            onAutoUpdate={(autoUpdate) => setDraft({ ...draft, autoUpdate })}
          />
          <section className="settings-section settings-backup">
            <div className="settings-label">
              <h3>{t("backupTitle")}</h3>
              <p>{t("backupHint")}</p>
            </div>
            <div className="backup-actions">
              <button type="button" onClick={onExport} disabled={saving}>
                <span className="backup-icon">
                  <ArrowUpFromLine size={17} />
                </span>
                <span>
                  <strong>{t("export")}</strong>
                  <small>{t("backupExportHint")}</small>
                </span>
                <ChevronRight size={16} />
              </button>
              <button type="button" onClick={onImport} disabled={saving}>
                <span className="backup-icon">
                  <ArrowDownToLine size={17} />
                </span>
                <span>
                  <strong>{t("import")}</strong>
                  <small>{t("backupImportHint")}</small>
                </span>
                <ChevronRight size={16} />
              </button>
            </div>
            <p className="hint">{t("backupMergeHint")}</p>
          </section>
        </div>
        <div className="dialog-footer settings-footer">
          <Button
            type="button"
            variant="outline"
            disabled={saving}
            onClick={requestClose}
          >
            {t("cancel")}
          </Button>
          <Button type="submit" disabled={saving}>
            {t("save")}
          </Button>
        </div>
      </form>
      <DiscardDialog
        open={discard}
        onKeep={() => setDiscard(false)}
        onDiscard={onClose}
      />
    </Dialog>
  );
}

function AuthDialog({
  challenge,
  respond,
}: {
  challenge: Challenge;
  respond: (value: string, accepted: boolean) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(""),
    [busy, setBusy] = useState(false);
  const host = challenge.kind === "host";
  async function submit(accepted: boolean) {
    if (busy) return;
    setBusy(true);
    const secret = value;
    setValue("");
    try {
      await respond(secret, accepted);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) void submit(false);
      }}
      title={t(
        host
          ? "hostTitle"
          : challenge.kind === "passphrase"
            ? "passphraseTitle"
            : "passwordTitle",
      )}
      description={t(host ? "hostHint" : "secretHint")}
    >
      <div className="auth-name">
        <KeyRound size={17} />
        {challenge.name}
      </div>
      <pre className="auth-prompt">{challenge.prompt}</pre>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void submit(true);
        }}
      >
        {!host && (
          <Field
            label={t(
              challenge.kind === "passphrase" ? "passphraseTitle" : "password",
            )}
          >
            <PasswordInput
              value={value}
              onChange={setValue}
              autoFocus
              required
            />
          </Field>
        )}
        <div className="dialog-footer">
          <Button
            type="button"
            variant="outline"
            disabled={busy}
            onClick={() => void submit(false)}
          >
            {t("deny")}
          </Button>
          <Button type="submit" disabled={busy}>
            {t(host ? "trust" : "continue")}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function CommandPreview({
  tunnel,
  actual,
  report,
}: {
  tunnel: Tunnel;
  actual?: string;
  report: (e: unknown) => void;
}) {
  const { t } = useTranslation();
  const [command, setCommand] = useState("");
  const [copied, setCopied] = useState(false);
  const serialized = JSON.stringify(tunnel);
  useEffect(() => {
    if (actual) {
      setCommand(actual);
      return;
    }
    let disposed = false;
    setCommand("");
    const timer = setTimeout(() => {
      if (desktop)
        void call<string>("PreviewCommand", JSON.parse(serialized))
          .then((value) => {
            if (!disposed) setCommand(value);
          })
          .catch(() => {
            if (!disposed) setCommand("");
          });
    }, 200);
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, [serialized, actual]);
  useEffect(() => {
    setCopied(false);
  }, [command]);
  return (
    <details className="command-preview">
      <summary>
        <Terminal size={15} />
        <span>{t("sshCommand")}</span>
        <ChevronRight size={14} />
      </summary>
      <div className="command-content">
        <p className="hint">{t("sshCommandHint")}</p>
        {command ? (
          <>
            <pre>
              <code>{command}</code>
            </pre>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() =>
                void navigator.clipboard
                  .writeText(command)
                  .then(() => setCopied(true))
                  .catch(report)
              }
            >
              {copied ? <Check size={13} /> : <Copy size={13} />}
              {t(copied ? "copied" : "copyCommand")}
            </Button>
          </>
        ) : (
          <p className="hint">{t("completeCommand")}</p>
        )}
      </div>
    </details>
  );
}

function DiscardDialog({
  open,
  onKeep,
  onDiscard,
}: {
  open: boolean;
  onKeep: () => void;
  onDiscard: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog
      open={open}
      onOpenChange={(value) => {
        if (!value) onKeep();
      }}
      title={t("discardTitle")}
      description={t("discardHint")}
    >
      <div className="dialog-footer">
        <Button variant="outline" onClick={onKeep}>
          {t("keepEditing")}
        </Button>
        <Button variant="destructive" onClick={onDiscard}>
          {t("discardChanges")}
        </Button>
      </div>
    </Dialog>
  );
}
