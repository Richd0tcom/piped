/**
 * Shipyard dummy data layer.
 *
 * To REMOVE all dummy data:
 *   1. Delete this file (src/mocks/shipyard-mock.ts)
 *   2. Delete the `import "@/mocks/shipyard-mock"` line in src/routes/index.tsx
 *
 * Nothing else in the app touches mocks. This module patches window.fetch
 * and window.EventSource only for URLs matching the Shipyard API. Other
 * requests pass through.
 */

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

const API_BASE =
  (import.meta.env.VITE_API_URL as string | undefined) ?? "http://localhost:8080";

const now = Date.now();
const iso = (offsetMs: number) => new Date(now - offsetMs).toISOString();

// ---------- state-change pub/sub for SSE ----------
type StateEvent =
  | { type: "snapshot"; deployments: Deployment[] }
  | { type: "created"; deployment: Deployment }
  | { type: "updated"; deployment: Deployment }
  | { type: "deleted"; id: string };

const stateSubscribers = new Set<(ev: StateEvent) => void>();
function broadcast(ev: StateEvent) {
  stateSubscribers.forEach((fn) => fn(ev));
}
function emitUpdated(d: Deployment) {
  d.updated_at = new Date().toISOString();
  broadcast({ type: "updated", deployment: { ...d } });
}

const deployments: Deployment[] = [
  {
    id: "dep_01",
    name: "api-gateway",
    source_type: "git",
    git_url: "https://github.com/acme/api-gateway.git",
    git_commit: "a3f9c21",
    git_commit_message: "fix: handle null user agent in request logger",
    image_tag: "api-gateway:a3f9c21",
    status: "running",
    caddy_route: "https://api.shipyard.local",
    port: 8081,
    created_at: iso(1000 * 60 * 47),
    updated_at: iso(1000 * 60 * 12),
  },
  {
    id: "dep_02",
    name: "web-frontend",
    source_type: "git",
    git_url: "https://github.com/acme/web.git",
    git_commit: "f12bd44",
    git_commit_message: "feat: dark mode toggle in settings page",
    image_tag: "web-frontend:f12bd44",
    status: "building",
    created_at: iso(1000 * 60 * 2),
    updated_at: iso(1000 * 30),
  },
  {
    id: "dep_03",
    name: "worker-billing",
    source_type: "upload",
    image_tag: "worker-billing:2025-04-23",
    status: "deploying",
    created_at: iso(1000 * 60 * 5),
    updated_at: iso(1000 * 15),
  },
  {
    id: "dep_04",
    name: "auth-service",
    source_type: "git",
    git_url: "https://github.com/acme/auth.git",
    git_commit: "9c1e7b0",
    git_commit_message: "refactor: extract jwt validation into middleware",
    image_tag: "auth-service:9c1e7b0",
    status: "failed",
    created_at: iso(1000 * 60 * 60 * 3),
    updated_at: iso(1000 * 60 * 60 * 2),
  },
  {
    id: "dep_05",
    name: "image-resizer",
    source_type: "git",
    git_url: "https://github.com/acme/resizer.git",
    git_commit: "7820ab1",
    git_commit_message: "perf: switch to libvips for 3x faster thumbnails",
    image_tag: "image-resizer:7820ab1",
    status: "rolled_back",
    caddy_route: "https://img.shipyard.local",
    port: 8082,
    created_at: iso(1000 * 60 * 60 * 24),
    updated_at: iso(1000 * 60 * 60 * 6),
  },
  {
    id: "dep_06",
    name: "metrics-collector",
    source_type: "git",
    git_url: "https://github.com/acme/metrics.git",
    git_commit: "ee4410d",
    git_commit_message: "chore: bump prometheus client to v1.9.0",
    status: "pending",
    created_at: iso(1000 * 30),
    updated_at: iso(1000 * 30),
  },
];

const logFixtures: Record<string, Omit<LogLine, "id" | "created_at">[]> = {
  dep_01: [
    { deployment_id: "dep_01", stream: "system", text: "[shipyard] container started", sequence: 1 },
    { deployment_id: "dep_01", stream: "stdout", text: "listening on :8081", sequence: 2 },
    { deployment_id: "dep_01", stream: "stdout", text: "GET /healthz 200 1ms", sequence: 3 },
    { deployment_id: "dep_01", stream: "stdout", text: "GET /v1/users 200 14ms", sequence: 4 },
    { deployment_id: "dep_01", stream: "stdout", text: "POST /v1/orders 201 32ms", sequence: 5 },
    { deployment_id: "dep_01", stream: "stderr", text: "warn: slow query (210ms): SELECT * FROM orders", sequence: 6 },
    { deployment_id: "dep_01", stream: "stdout", text: "GET /v1/users/42 200 8ms", sequence: 7 },
  ],
  dep_02: [
    { deployment_id: "dep_02", stream: "system", text: "[shipyard] cloning https://github.com/acme/web.git", sequence: 1 },
    { deployment_id: "dep_02", stream: "system", text: "[shipyard] checked out f12bd44", sequence: 2 },
    { deployment_id: "dep_02", stream: "stdout", text: "Step 1/8 : FROM node:20-alpine", sequence: 3 },
    { deployment_id: "dep_02", stream: "stdout", text: "Step 2/8 : WORKDIR /app", sequence: 4 },
    { deployment_id: "dep_02", stream: "stdout", text: "Step 3/8 : COPY package*.json ./", sequence: 5 },
    { deployment_id: "dep_02", stream: "stdout", text: "Step 4/8 : RUN npm ci", sequence: 6 },
    { deployment_id: "dep_02", stream: "stdout", text: "added 482 packages in 12s", sequence: 7 },
  ],
  dep_03: [
    { deployment_id: "dep_03", stream: "system", text: "[shipyard] image worker-billing:2025-04-23 pulled", sequence: 1 },
    { deployment_id: "dep_03", stream: "system", text: "[shipyard] starting container", sequence: 2 },
    { deployment_id: "dep_03", stream: "stdout", text: "billing worker booting...", sequence: 3 },
    { deployment_id: "dep_03", stream: "stdout", text: "connected to redis://queue:6379", sequence: 4 },
    { deployment_id: "dep_03", stream: "stdout", text: "waiting for jobs", sequence: 5 },
  ],
  dep_04: [
    { deployment_id: "dep_04", stream: "system", text: "[shipyard] build started", sequence: 1 },
    { deployment_id: "dep_04", stream: "stdout", text: "Step 1/6 : FROM golang:1.22", sequence: 2 },
    { deployment_id: "dep_04", stream: "stdout", text: "Step 2/6 : COPY . .", sequence: 3 },
    { deployment_id: "dep_04", stream: "stdout", text: "Step 3/6 : RUN go build -o auth ./cmd/auth", sequence: 4 },
    { deployment_id: "dep_04", stream: "stderr", text: "auth/jwt.go:42:9: undefined: jwt.ParseWithClaims", sequence: 5 },
    { deployment_id: "dep_04", stream: "stderr", text: "build failed with exit code 1", sequence: 6 },
    { deployment_id: "dep_04", stream: "system", text: "[shipyard] deployment failed", sequence: 7 },
  ],
  dep_05: [
    { deployment_id: "dep_05", stream: "system", text: "[shipyard] rolled back to image-resizer:7820ab1", sequence: 1 },
    { deployment_id: "dep_05", stream: "stdout", text: "image resizer ready on :8082", sequence: 2 },
    { deployment_id: "dep_05", stream: "stdout", text: "processed 142 jobs in last hour", sequence: 3 },
  ],
  dep_06: [
    { deployment_id: "dep_06", stream: "system", text: "[shipyard] queued for build", sequence: 1 },
  ],
};

// ---------- fetch interceptor ----------
const realFetch = window.fetch.bind(window);

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

function matchesApi(url: string): string | null {
  if (url.startsWith(`${API_BASE}/api/deployments`)) {
    return url.slice(API_BASE.length);
  }
  return null;
}

window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
  const url =
    typeof input === "string"
      ? input
      : input instanceof URL
        ? input.toString()
        : input.url;
  const path = matchesApi(url);
  if (!path) return realFetch(input, init);

  const method = (init?.method ?? "GET").toUpperCase();

  if (method === "GET" && path === "/api/deployments") {
    return jsonResponse(deployments);
  }

  const getOne = path.match(/^\/api\/deployments\/([^/]+)$/);
  if (method === "GET" && getOne) {
    const d = deployments.find((x) => x.id === getOne[1]);
    if (!d) return new Response("not found", { status: 404 });
    return jsonResponse(d);
  }

  const delOne = path.match(/^\/api\/deployments\/([^/]+)$/);
  if (method === "DELETE" && delOne) {
    const idx = deployments.findIndex((x) => x.id === delOne[1]);
    if (idx === -1) return new Response("not found", { status: 404 });
    deployments.splice(idx, 1);
    delete logFixtures[delOne[1]];
    broadcast({ type: "deleted", id: delOne[1] });
    return jsonResponse({ ok: true });
  }

  if (method === "POST" && path === "/api/deployments") {
    const id = `dep_${Math.random().toString(36).slice(2, 8)}`;
    let name = "new-deployment";
    let sourceType: "git" | "upload" = "git";
    let gitUrl: string | undefined;
    let gitCommit: string | undefined;

    const contentType = init?.headers
      ? (new Headers(init.headers).get("Content-Type") ?? "")
      : "";

    if (contentType.includes("application/json") && typeof init?.body === "string") {
      try {
        const body = JSON.parse(init.body);
        name = body.name ?? name;
        sourceType = body.source_type ?? sourceType;
        gitUrl = body.git_url;
        gitCommit = body.git_commit;
      } catch {
        /* ignore */
      }
    } else if (init?.body instanceof FormData) {
      name = (init.body.get("name") as string) ?? name;
      sourceType = ((init.body.get("source_type") as string) ?? "upload") as "git" | "upload";
    }

    const created: Deployment = {
      id,
      name,
      source_type: sourceType,
      git_url: gitUrl,
      git_commit: gitCommit,
      status: "pending",
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };
    deployments.unshift(created);
    logFixtures[id] = [
      { deployment_id: id, stream: "system", text: "[shipyard] queued for build", sequence: 1 },
    ];
    broadcast({ type: "created", deployment: { ...created } });
    setTimeout(() => {
      const d = deployments.find((x) => x.id === id);
      if (d) {
        d.status = "building";
        emitUpdated(d);
      }
    }, 1500);
    setTimeout(() => {
      const d = deployments.find((x) => x.id === id);
      if (d) {
        d.status = "running";
        d.image_tag = `${name}:latest`;
        d.caddy_route = `https://${name}.shipyard.local`;
        d.port = 8090;
        emitUpdated(d);
      }
    }, 6000);
    return jsonResponse(created, { status: 201 });
  }

  const restart = path.match(/^\/api\/deployments\/([^/]+)\/restart$/);
  if (method === "POST" && restart) {
    const d = deployments.find((x) => x.id === restart[1]);
    if (d) {
      d.status = "deploying";
      emitUpdated(d);
      setTimeout(() => {
        d.status = "running";
        emitUpdated(d);
      }, 2500);
    }
    return jsonResponse({ ok: true });
  }

  const rollback = path.match(/^\/api\/deployments\/([^/]+)\/rollback$/);
  if (method === "POST" && rollback) {
    const d = deployments.find((x) => x.id === rollback[1]);
    if (d && typeof init?.body === "string") {
      try {
        const body = JSON.parse(init.body);
        d.image_tag = body.image_tag ?? d.image_tag;
      } catch {
        /* ignore */
      }
      d.status = "rolled_back";
      emitUpdated(d);
    }
    return jsonResponse({ ok: true });
  }

  return new Response("mock: not implemented", { status: 501 });
};

// ---------- EventSource interceptor ----------
const RealEventSource = window.EventSource;

class MockEventSource {
  static readonly CONNECTING = 0 as const;
  static readonly OPEN = 1 as const;
  static readonly CLOSED = 2 as const;
  readonly CONNECTING = 0 as const;
  readonly OPEN = 1 as const;
  readonly CLOSED = 2 as const;

  url: string;
  readyState: number = 0;
  withCredentials = false;
  onopen: ((this: EventSource, ev: Event) => unknown) | null = null;
  onmessage: ((this: EventSource, ev: MessageEvent) => unknown) | null = null;
  onerror: ((this: EventSource, ev: Event) => unknown) | null = null;

  private timers: ReturnType<typeof setTimeout>[] = [];
  private interval: ReturnType<typeof setInterval> | null = null;
  private listeners: Record<string, Set<EventListener>> = {};
  private unsubscribe: (() => void) | null = null;

  constructor(url: string | URL) {
    this.url = url.toString();

    // SSE: deployment state-change stream
    if (this.url.includes("/api/deployments/events")) {
      this.readyState = 1;
      this.timers.push(
        setTimeout(() => {
          this.emitRaw({
            type: "snapshot",
            deployments: deployments.map((d) => ({ ...d })),
          });
        }, 0),
      );
      const sub = (ev: StateEvent) => this.emitRaw(ev);
      stateSubscribers.add(sub);
      this.unsubscribe = () => stateSubscribers.delete(sub);
      return;
    }

    const m = this.url.match(/\/api\/deployments\/([^/]+)\/logs/);
    if (!m) {
      const real = new RealEventSource(url);
      real.onmessage = (e) => this.dispatch("message", e);
      real.onerror = (e) => this.dispatch("error", e);
      return;
    }

    const id = m[1];
    this.readyState = 1;
    const lines = logFixtures[id] ?? [];
    let seq = lines.length;

    lines.forEach((line, i) => {
      this.timers.push(
        setTimeout(() => {
          this.emit({
            ...line,
            id: i + 1,
            created_at: new Date().toISOString(),
          });
        }, 80 * i),
      );
    });

    const dep = deployments.find((d) => d.id === id);
    if (
      dep &&
      (dep.status === "running" || dep.status === "building" || dep.status === "deploying")
    ) {
      const samples = [
        { stream: "stdout" as const, text: "GET /healthz 200 1ms" },
        { stream: "stdout" as const, text: "GET /v1/items 200 18ms" },
        { stream: "stdout" as const, text: "POST /v1/events 202 4ms" },
        { stream: "stderr" as const, text: "warn: cache miss for key=user:42" },
        { stream: "system" as const, text: "[shipyard] heartbeat ok" },
      ];
      this.interval = setInterval(() => {
        const s = samples[Math.floor(Math.random() * samples.length)];
        seq += 1;
        this.emit({
          id: seq,
          deployment_id: id,
          stream: s.stream,
          text: s.text,
          sequence: seq,
          created_at: new Date().toISOString(),
        });
      }, 1500);
    }
  }

  private emit(line: LogLine) {
    const ev = new MessageEvent("message", { data: JSON.stringify(line) });
    this.onmessage?.call(this as unknown as EventSource, ev);
    this.dispatch("message", ev);
  }

  private emitRaw(payload: unknown) {
    const ev = new MessageEvent("message", { data: JSON.stringify(payload) });
    this.onmessage?.call(this as unknown as EventSource, ev);
    this.dispatch("message", ev);
  }

  private dispatch(type: string, ev: Event) {
    this.listeners[type]?.forEach((l) => l.call(this as unknown as EventTarget, ev));
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    (this.listeners[type] ??= new Set()).add(listener as EventListener);
  }
  removeEventListener(type: string, listener: EventListenerOrEventListenerObject) {
    this.listeners[type]?.delete(listener as EventListener);
  }
  dispatchEvent(ev: Event): boolean {
    this.dispatch(ev.type, ev);
    return true;
  }

  close() {
    this.readyState = 2;
    this.timers.forEach((t) => clearTimeout(t));
    this.timers = [];
    if (this.interval) clearInterval(this.interval);
    this.interval = null;
    this.unsubscribe?.();
    this.unsubscribe = null;
  }
}

window.EventSource = MockEventSource as unknown as typeof EventSource;

// eslint-disable-next-line no-console
console.info(
  "[shipyard] using dummy data. Remove src/mocks/shipyard-mock.ts and its import in src/routes/index.tsx to disable.",
);
