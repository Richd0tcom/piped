import { createFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent } from "react";

// DUMMY DATA: remove this import and delete src/mocks/shipyard-mock.ts to use real API.
// if (typeof window !== "undefined") {
//   void import("@/mocks/shipyard-mock");
// }

export const Route = createFileRoute("/")({
  component: ShipyardDashboard,
});

// ---------- Types ----------
type DeploymentStatus =
  | "pending"
  | "building"
  | "deploying"
  | "running"
  | "failed"
  | "rolled_back";

type Deployment = {
  id: string;
  name: string;
  source_type: "git" | "upload";
  git_url?: string;
  git_commit?: string;
  git_commit_message?: string;
  image_tag?: string;
  status: DeploymentStatus;
  caddy_route?: string;
  port?: number;
  created_at: string;
  updated_at: string;
};

type LogLine = {
  id: number;
  deployment_id: string;
  stream: "stdout" | "stderr" | "system";
  text: string;
  sequence: number;
  created_at: string;
};

// const API_BASE =
//   (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8080";

const API_BASE = ""

// ---------- Helpers ----------
const STATUS_STYLE: Record<DeploymentStatus, { bg: string; fg: string; pulse?: boolean }> = {
  pending: { bg: "#3a3a40", fg: "#cfcfd4" },
  building: { bg: "#5c4711", fg: "#ffd66b", pulse: true },
  deploying: { bg: "#0f3a66", fg: "#7ec1ff", pulse: true },
  running: { bg: "#10401f", fg: "#5fe39a" },
  failed: { bg: "#4a1414", fg: "#ff7676" },
  rolled_back: { bg: "#3a1a4a", fg: "#c98bff" },
};

function StatusBadge({ status }: { status: DeploymentStatus }) {
  const s = STATUS_STYLE[status];
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 999,
        fontSize: 11,
        fontWeight: 600,
        textTransform: "uppercase",
        letterSpacing: 0.5,
        background: s.bg,
        color: s.fg,
        animation: s.pulse ? "shipyard-pulse 1.4s ease-in-out infinite" : undefined,
      }}
    >
      {status.replace("_", " ")}
    </span>
  );
}

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime();
  const diff = Math.max(0, Date.now() - then);
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

// ---------- Component ----------
function ShipyardDashboard() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<Deployment | null>(null);
  const queryClient = useQueryClient();

  const deploymentsQuery = useQuery<Deployment[]>({
    queryKey: ["deployments"],
    queryFn: async () => {
      const r = await fetch(`${API_BASE}/api/deployments`);
      if (!r.ok) throw new Error("Failed to load deployments");
      return r.json();
    },
  });

  const selectedQuery = useQuery<Deployment>({
    queryKey: ["deployment", selectedId],
    queryFn: async () => {
      const r = await fetch(`${API_BASE}/api/deployments/${selectedId}`);
      if (!r.ok) throw new Error("Failed to load deployment");
      return r.json();
    },
    enabled: !!selectedId,
  });

  // SSE: live deployment state changes (replaces polling)
  useEffect(() => {
    const es = new EventSource(`${API_BASE}/api/deployments/events`);
    es.onmessage = (e) => {
      let msg:
        | { type: "snapshot"; deployments: Deployment[] }
        | { type: "created"; deployment: Deployment }
        | { type: "updated"; deployment: Deployment }
        | { type: "deleted"; id: string };
      try {
        msg = JSON.parse(e.data);
      } catch {
        return;
      }

      if (msg.type === "snapshot") {
        queryClient.setQueryData<Deployment[]>(["deployments"], msg.deployments);
        return;
      }

      if (msg.type === "deleted") {
        queryClient.setQueryData<Deployment[]>(["deployments"], (prev) =>
          (prev ?? []).filter((d) => d.id !== msg.id),
        );
        queryClient.removeQueries({ queryKey: ["deployment", msg.id] });
        return;
      }

      // created or updated
      const incoming = msg.deployment;
      queryClient.setQueryData<Deployment[]>(["deployments"], (prev) => {
        const list = prev ?? [];
        const idx = list.findIndex((d) => d.id === incoming.id);
        if (idx === -1) return [incoming, ...list];
        const next = list.slice();
        next[idx] = incoming;
        return next;
      });
      queryClient.setQueryData<Deployment>(["deployment", incoming.id], incoming);
    };
    es.onerror = () => {
      // EventSource auto-reconnects; no-op
    };
    return () => es.close();
  }, [queryClient]);

  const restartMutation = useMutation({
    mutationFn: async (id: string) => {
      const r = await fetch(`${API_BASE}/api/deployments/${id}/restart`, { method: "POST" });
      if (!r.ok) throw new Error("Restart failed");
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["deployments"] }),
  });

  const rollbackMutation = useMutation({
    mutationFn: async ({ id, image_tag }: { id: string; image_tag: string }) => {
      const r = await fetch(`${API_BASE}/api/deployments/${id}/rollback`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ image_tag }),
      });
      if (!r.ok) throw new Error("Rollback failed");
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["deployments"] }),
  });

  const deleteMutation = useMutation({
    mutationFn: async (id: string) => {
      const r = await fetch(`${API_BASE}/api/deployments/${id}`, { method: "DELETE" });
      if (!r.ok) throw new Error("Delete failed");
      return id;
    },
    onSuccess: (id) => {
      if (selectedId === id) setSelectedId(null);
      queryClient.invalidateQueries({ queryKey: ["deployments"] });
    },
  });

  const deployments = deploymentsQuery.data ?? [];
  const selected = selectedQuery.data;

  return (
    <div style={styles.app}>
      <style>{globalCss}</style>

      {/* Header */}
      <header style={styles.header}>
        <div style={styles.brand}>
          <span style={styles.brandMark}>⚓</span>
          <span style={styles.brandName}>Shipyard</span>
        </div>
        <button style={styles.primaryBtn} onClick={() => setModalOpen(true)}>
          + New Deployment
        </button>
      </header>

      {/* Body */}
      <main style={styles.main}>
        <section style={styles.listPane}>
          <div style={styles.paneHeader}>
            <h2 style={styles.paneTitle}>Deployments</h2>
            <span style={styles.paneCount}>{deployments.length}</span>
          </div>
          <div style={styles.listScroll}>
            {deploymentsQuery.isLoading && (
              <div style={styles.empty}>Loading...</div>
            )}
            {!deploymentsQuery.isLoading && deployments.length === 0 && (
              <div style={styles.empty}>No deployments yet</div>
            )}
            {deployments.map((d) => (
              <DeploymentRow
                key={d.id}
                deployment={d}
                selected={d.id === selectedId}
                onSelect={() => setSelectedId(d.id)}
                onRestart={() => restartMutation.mutate(d.id)}
                onRollback={() => {
                  const tag = window.prompt("Rollback to image tag:", d.image_tag ?? "");
                  if (tag) rollbackMutation.mutate({ id: d.id, image_tag: tag });
                }}
                onDelete={() => {
                  setConfirmDelete(d);
                }}
              />
            ))}
          </div>
        </section>

        <section style={styles.logPane}>
          {selected ? (
            <LogPanel deployment={selected} />
          ) : selectedId ? (
            <div style={styles.empty}>Loading deployment...</div>
          ) : (
            <div style={styles.empty}>Select a deployment to view logs</div>
          )}
        </section>
      </main>

      {modalOpen && (
        <NewDeploymentModal
          onClose={() => setModalOpen(false)}
          onCreated={() => {
            setModalOpen(false);
            queryClient.invalidateQueries({ queryKey: ["deployments"] });
          }}
        />
      )}

      {confirmDelete && (
        <ConfirmDeleteModal
          deployment={confirmDelete}
          loading={deleteMutation.isPending}
          onCancel={() => setConfirmDelete(null)}
          onConfirm={() => {
            const id = confirmDelete.id;
            deleteMutation.mutate(id, {
              onSettled: () => setConfirmDelete(null),
            });
          }}
        />
      )}
    </div>
  );
}

// ---------- Confirm delete modal ----------
function ConfirmDeleteModal({
  deployment,
  loading,
  onCancel,
  onConfirm,
}: {
  deployment: Deployment;
  loading: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div style={styles.overlay} onClick={onCancel}>
      <div
        style={{ ...styles.modal, maxWidth: 440 }}
        onClick={(e) => e.stopPropagation()}
      >
        <div style={styles.modalHeader}>
          <h2 style={styles.modalTitle}>Delete deployment</h2>
          <button style={styles.closeBtn} onClick={onCancel} aria-label="Close">
            ×
          </button>
        </div>
        <div style={{ padding: "0 20px 8px", color: "#cfcfd4", fontSize: 14, lineHeight: 1.5 }}>
          Are you sure you want to delete{" "}
          <strong style={{ color: "#fff" }}>{deployment.name}</strong>? This action
          cannot be undone and its logs will be removed.
        </div>
        <div
          style={{
            display: "flex",
            justifyContent: "flex-end",
            gap: 8,
            padding: 20,
          }}
        >
          <button
            type="button"
            style={styles.secondaryBtn}
            onClick={onCancel}
            disabled={loading}
          >
            Cancel
          </button>
          <button
            type="button"
            style={styles.dangerBtn}
            onClick={onConfirm}
            disabled={loading}
          >
            {loading ? "Deleting..." : "Delete"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------- Deployment row ----------
function DeploymentRow({
  deployment,
  selected,
  onSelect,
  onRestart,
  onRollback,
  onDelete,
}: {
  deployment: Deployment;
  selected: boolean;
  onSelect: () => void;
  onRestart: () => void;
  onRollback: () => void;
  onDelete: () => void;
}) {
  const showActions = deployment.status === "running" || deployment.status === "rolled_back";
  return (
    <div
      onClick={onSelect}
      style={{
        ...styles.row,
        ...(selected ? styles.rowSelected : {}),
      }}
    >
      <div style={styles.rowMain}>
        <div style={styles.rowTop}>
          <span style={styles.rowName}>{deployment.name}</span>
          <StatusBadge status={deployment.status} />
        </div>
        {(deployment.git_commit || deployment.git_commit_message) && (
          <div style={styles.rowCommit}>
            {deployment.git_commit && (
              <code style={styles.commitHash}>{deployment.git_commit.slice(0, 7)}</code>
            )}
            {deployment.git_commit_message && (
              <span style={styles.commitMsg}>{deployment.git_commit_message}</span>
            )}
          </div>
        )}
        <div style={styles.rowMeta}>
          {deployment.image_tag && (
            <span style={styles.tag}>{deployment.image_tag}</span>
          )}
          {deployment.caddy_route && deployment.status === "running" && (
            <a
              href={deployment.caddy_route}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => e.stopPropagation()}
              style={styles.routeLink}
            >
              {deployment.caddy_route}
            </a>
          )}
          <span style={styles.timeStamp}>{relativeTime(deployment.created_at)}</span>
        </div>
      </div>
      <div style={styles.rowActions}>
        {showActions && (
          <>
            <button
              title="Restart"
              style={styles.iconBtn}
              onClick={(e) => {
                e.stopPropagation();
                onRestart();
              }}
            >
              ⟳
            </button>
            <button
              title="Rollback"
              style={styles.iconBtn}
              onClick={(e) => {
                e.stopPropagation();
                onRollback();
              }}
            >
              ↩
            </button>
          </>
        )}
        <button
          title="Delete"
          style={{ ...styles.iconBtn, ...styles.iconBtnDanger }}
          onClick={(e) => {
            e.stopPropagation();
            onDelete();
          }}
        >
          ✕
        </button>
      </div>
    </div>
  );
}

// ---------- Log panel (SSE) ----------
function LogPanel({ deployment }: { deployment: Deployment }) {
  const [logs, setLogs] = useState<LogLine[]>([]);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setLogs([]);
    const es = new EventSource(`${API_BASE}/api/deployments/${deployment.id}/logs`);
    es.onmessage = (e) => {
      try {
        const line: LogLine = JSON.parse(e.data);
        setLogs((prev) => [...prev, line]);
      } catch {
        /* ignore malformed line */
      }
    };
    es.onerror = () => es.close();
    return () => es.close();
  }, [deployment.id]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [logs]);

  return (
    <>
      <div style={styles.paneHeader}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <h2 style={styles.paneTitle}>{deployment.name}</h2>
          <StatusBadge status={deployment.status} />
          {deployment.image_tag && <span style={styles.tag}>{deployment.image_tag}</span>}
          {deployment.status === "running" && deployment.caddy_route && (
            <a
              href={deployment.caddy_route}
              target="_blank"
              rel="noreferrer"
              style={styles.routeLink}
            >
              ↗ {deployment.caddy_route}
            </a>
          )}
        </div>
      </div>
      <pre style={styles.logScroll}>
        {logs.length === 0 && (
          <div style={{ color: "#6b7280" }}>Waiting for log output...</div>
        )}
        {logs.map((l) => (
          <div
            key={l.id}
            style={{
              color:
                l.stream === "stderr"
                  ? "#ff8b6b"
                  : l.stream === "system"
                    ? "#5fd6d6"
                    : "#e6e6ea",
              whiteSpace: "pre-wrap",
              wordBreak: "break-word",
            }}
          >
            {l.text}
          </div>
        ))}
        <div ref={endRef} />
      </pre>
    </>
  );
}

// ---------- New Deployment modal ----------
type EnvRow = { id: number; key: string; value: string };

function NewDeploymentModal({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [sourceType, setSourceType] = useState<"git" | "upload">("git");
  const [gitUrl, setGitUrl] = useState("");
  const [gitCommit, setGitCommit] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [envRows, setEnvRows] = useState<EnvRow[]>([{ id: 0, key: "", value: "" }]);
  const nextEnvId = useRef(1);
  const [error, setError] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: async () => {
      const envVars = envRows.reduce<Record<string, string>>((acc, r) => {
        if (r.key.trim()) acc[r.key.trim()] = r.value;
        return acc;
      }, {});

      if (sourceType === "git") {
        const r = await fetch(`${API_BASE}/api/deployments`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            name,
            source_type: "git",
            git_url: gitUrl,
            git_commit: gitCommit || undefined,
            env_vars: envVars,
          }),
        });
        if (!r.ok) throw new Error(await r.text());
      } else {
        if (!file) throw new Error("Please choose an archive file");
        const fd = new FormData();
        fd.append("name", name);
        fd.append("source_type", "upload");
        fd.append("env_vars", JSON.stringify(envVars));
        fd.append("archive", file);
        const r = await fetch(`${API_BASE}/api/deployments`, {
          method: "POST",
          body: fd,
        });
        if (!r.ok) throw new Error(await r.text());
      }
    },
    onSuccess: onCreated,
    onError: (e: unknown) => setError(e instanceof Error ? e.message : "Failed"),
  });

  const addRow = () =>
    setEnvRows((prev) => [...prev, { id: nextEnvId.current++, key: "", value: "" }]);
  const removeRow = (id: number) =>
    setEnvRows((prev) => (prev.length > 1 ? prev.filter((r) => r.id !== id) : prev));
  const updateRow = (id: number, patch: Partial<EnvRow>) =>
    setEnvRows((prev) => prev.map((r) => (r.id === id ? { ...r, ...patch } : r)));

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!name.trim()) {
      setError("Name is required");
      return;
    }
    if (sourceType === "git" && !gitUrl.trim()) {
      setError("Git URL is required");
      return;
    }
    createMutation.mutate();
  };

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div style={styles.modalHeader}>
          <h2 style={styles.modalTitle}>New Deployment</h2>
          <button style={styles.closeBtn} onClick={onClose} aria-label="Close">
            ×
          </button>
        </div>
        <form onSubmit={submit} style={styles.form}>
          <label style={styles.label}>
            Name
            <input
              style={styles.input}
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              placeholder="my-service"
            />
          </label>

          <div style={styles.label}>
            Source
            <div style={{ display: "flex", gap: 16, marginTop: 6 }}>
              <label style={styles.radioLabel}>
                <input
                  type="radio"
                  checked={sourceType === "git"}
                  onChange={() => setSourceType("git")}
                />
                Git URL
              </label>
              <label style={styles.radioLabel}>
                <input
                  type="radio"
                  checked={sourceType === "upload"}
                  onChange={() => setSourceType("upload")}
                />
                Upload Archive
              </label>
            </div>
          </div>

          {sourceType === "git" ? (
            <>
              <label style={styles.label}>
                Git URL
                <input
                  style={styles.input}
                  value={gitUrl}
                  onChange={(e) => setGitUrl(e.target.value)}
                  required
                  placeholder="https://github.com/user/repo.git"
                />
              </label>
              <label style={styles.label}>
                Git Commit
                <input
                  style={styles.input}
                  value={gitCommit}
                  onChange={(e) => setGitCommit(e.target.value)}
                  placeholder="HEAD"
                />
              </label>
            </>
          ) : (
            <label style={styles.label}>
              Archive
              <input
                style={{ ...styles.input, padding: 6 }}
                type="file"
                accept=".zip,.tar.gz,.tgz"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
            </label>
          )}

          <div style={styles.label}>
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                marginBottom: 6,
              }}
            >
              <span>Environment Variables</span>
              <button type="button" style={styles.ghostBtn} onClick={addRow}>
                + Add variable
              </button>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              {envRows.map((row) => (
                <div key={row.id} style={{ display: "flex", gap: 6 }}>
                  <input
                    style={{ ...styles.input, flex: 1 }}
                    placeholder="KEY"
                    value={row.key}
                    onChange={(e) => updateRow(row.id, { key: e.target.value })}
                  />
                  <input
                    style={{ ...styles.input, flex: 2 }}
                    placeholder="value"
                    value={row.value}
                    onChange={(e) => updateRow(row.id, { value: e.target.value })}
                  />
                  <button
                    type="button"
                    style={styles.iconBtn}
                    onClick={() => removeRow(row.id)}
                    aria-label="Remove"
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
          </div>

          {error && <div style={styles.errorBox}>{error}</div>}

          <div style={styles.modalActions}>
            <button type="button" style={styles.ghostBtn} onClick={onClose}>
              Cancel
            </button>
            <button
              type="submit"
              style={{ ...styles.primaryBtn, opacity: createMutation.isPending ? 0.7 : 1 }}
              disabled={createMutation.isPending}
            >
              {createMutation.isPending ? "Deploying..." : "Deploy"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

// ---------- Styles ----------
const globalCss = `
  html, body, #root { height: 100%; margin: 0; }
  body {
    background: #0b0c10;
    color: #e6e6ea;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
  }
  * { box-sizing: border-box; }
  @keyframes shipyard-pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.45; }
  }
  ::-webkit-scrollbar { width: 10px; height: 10px; }
  ::-webkit-scrollbar-track { background: transparent; }
  ::-webkit-scrollbar-thumb { background: #2a2c33; border-radius: 6px; }
  ::-webkit-scrollbar-thumb:hover { background: #3a3d45; }
`;

const styles = {
  app: {
    minHeight: "100vh",
    display: "flex",
    flexDirection: "column",
    background: "#0b0c10",
  },
  header: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    padding: "14px 24px",
    borderBottom: "1px solid #1f2128",
    background: "#0f1015",
  },
  brand: { display: "flex", alignItems: "center", gap: 10 },
  brandMark: { fontSize: 20 },
  brandName: { fontSize: 18, fontWeight: 700, letterSpacing: 0.5 },
  primaryBtn: {
    background: "#3b82f6",
    color: "#fff",
    border: "none",
    padding: "8px 14px",
    borderRadius: 6,
    fontSize: 13,
    fontWeight: 600,
    cursor: "pointer",
  },
  secondaryBtn: {
    background: "#1a1c22",
    color: "#cfcfd4",
    border: "1px solid #2a2c33",
    padding: "8px 14px",
    borderRadius: 6,
    fontSize: 13,
    fontWeight: 600,
    cursor: "pointer",
  },
  dangerBtn: {
    background: "#dc2626",
    color: "#fff",
    border: "none",
    padding: "8px 14px",
    borderRadius: 6,
    fontSize: 13,
    fontWeight: 600,
    cursor: "pointer",
  },
  main: {
    flex: 1,
    display: "grid",
    gridTemplateColumns: "minmax(320px, 40%) 1fr",
    gap: 1,
    background: "#1f2128",
    minHeight: 0,
  },
  listPane: {
    background: "#0f1015",
    display: "flex",
    flexDirection: "column",
    minHeight: 0,
  },
  logPane: {
    background: "#0f1015",
    display: "flex",
    flexDirection: "column",
    minHeight: 0,
  },
  paneHeader: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    padding: "14px 18px",
    borderBottom: "1px solid #1f2128",
    gap: 12,
  },
  paneTitle: { margin: 0, fontSize: 14, fontWeight: 600, color: "#e6e6ea" },
  paneCount: {
    fontSize: 12,
    color: "#9ca3af",
    background: "#1a1c22",
    padding: "2px 8px",
    borderRadius: 999,
  },
  listScroll: { overflowY: "auto", flex: 1 },
  empty: {
    padding: 32,
    textAlign: "center",
    color: "#6b7280",
    fontSize: 13,
  },
  row: {
    display: "flex",
    alignItems: "center",
    gap: 10,
    padding: "12px 16px",
    borderBottom: "1px solid #15171d",
    cursor: "pointer",
    transition: "background 0.12s",
  },
  rowSelected: { background: "#172033", borderLeft: "3px solid #3b82f6", paddingLeft: 13 },
  rowMain: { flex: 1, minWidth: 0 },
  rowTop: { display: "flex", alignItems: "center", gap: 10, marginBottom: 4 },
  rowName: { fontSize: 13, fontWeight: 600, color: "#e6e6ea" },
  rowCommit: {
    display: "flex",
    alignItems: "baseline",
    gap: 8,
    marginBottom: 4,
    minWidth: 0,
  },
  commitHash: {
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
    fontSize: 11,
    color: "#7ec1ff",
    background: "#0f3a66",
    padding: "1px 5px",
    borderRadius: 3,
    flexShrink: 0,
  },
  commitMsg: {
    fontSize: 12,
    color: "#cfcfd4",
    overflow: "hidden",
    textOverflow: "ellipsis",
    whiteSpace: "nowrap",
    minWidth: 0,
  },
  rowMeta: {
    display: "flex",
    alignItems: "center",
    gap: 10,
    flexWrap: "wrap",
    fontSize: 11,
    color: "#9ca3af",
  },
  tag: {
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
    background: "#1a1c22",
    padding: "2px 6px",
    borderRadius: 4,
    color: "#cfcfd4",
    fontSize: 11,
  },
  routeLink: {
    color: "#7ec1ff",
    textDecoration: "none",
    fontSize: 11,
  },
  timeStamp: { color: "#6b7280" },
  rowActions: { display: "flex", gap: 4 },
  iconBtn: {
    background: "#1a1c22",
    color: "#cfcfd4",
    border: "1px solid #2a2c33",
    width: 28,
    height: 28,
    borderRadius: 6,
    cursor: "pointer",
    fontSize: 14,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
  },
  iconBtnDanger: {
    background: "#2a1414",
    color: "#ff7676",
    border: "1px solid #4a1a1a",
  },
  logScroll: {
    flex: 1,
    margin: 0,
    padding: 16,
    background: "#06070a",
    overflow: "auto",
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
    fontSize: 12.5,
    lineHeight: 1.55,
    minHeight: 0,
  },
  overlay: {
    position: "fixed",
    inset: 0,
    background: "rgba(0,0,0,0.6)",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    zIndex: 50,
    padding: 20,
  },
  modal: {
    background: "#13141a",
    border: "1px solid #2a2c33",
    borderRadius: 10,
    width: "100%",
    maxWidth: 520,
    maxHeight: "90vh",
    display: "flex",
    flexDirection: "column",
    overflow: "hidden",
  },
  modalHeader: {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    padding: "14px 20px",
    borderBottom: "1px solid #1f2128",
  },
  modalTitle: { margin: 0, fontSize: 16, fontWeight: 600 },
  closeBtn: {
    background: "transparent",
    color: "#9ca3af",
    border: "none",
    fontSize: 22,
    cursor: "pointer",
    lineHeight: 1,
  },
  form: {
    display: "flex",
    flexDirection: "column",
    gap: 14,
    padding: 20,
    overflowY: "auto",
  },
  label: { display: "flex", flexDirection: "column", gap: 4, fontSize: 12, color: "#cfcfd4" },
  input: {
    background: "#0b0c10",
    border: "1px solid #2a2c33",
    color: "#e6e6ea",
    padding: "8px 10px",
    borderRadius: 6,
    fontSize: 13,
    outline: "none",
    fontFamily: "inherit",
  },
  radioLabel: {
    display: "flex",
    alignItems: "center",
    gap: 6,
    fontSize: 13,
    color: "#e6e6ea",
    cursor: "pointer",
  },
  ghostBtn: {
    background: "transparent",
    color: "#cfcfd4",
    border: "1px solid #2a2c33",
    padding: "6px 10px",
    borderRadius: 6,
    fontSize: 12,
    cursor: "pointer",
  },
  modalActions: {
    display: "flex",
    justifyContent: "flex-end",
    gap: 8,
    paddingTop: 6,
  },
  errorBox: {
    background: "#3a1313",
    border: "1px solid #5a1f1f",
    color: "#ff9999",
    padding: "8px 10px",
    borderRadius: 6,
    fontSize: 12,
  },
} satisfies Record<string, React.CSSProperties>;
