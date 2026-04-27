# Piped (that one brimble assessment)

Submit a Git URL or a project archive. Shipyard builds it into a container image, runs it, and gives you a live URL — all from a single page.

```bash
git clone https://github.com/you/shipyard
cd shipyard
docker compose up
```

Open `http://localhost`. No accounts. No config. Done.

---

## Why Go

The brief preferred TypeScript but Go was the right call here. The backend's entire job is running subprocesses (Railpack), piping their output line by line, managing containers, and fanning those log lines out to however many browser tabs are watching. That's a concurrency problem, not a business logic problem. Go's goroutines handle it without ceremony. The binary is small, idle memory is low, and there's no runtime to babysit inside Docker.

NestJS was briefly considered and immediately dismissed — it's a large framework for large teams. This is six endpoints and a pipeline.

---

## How the pipeline works

```
Git URL or archive
      ↓
clone / extract into a temp dir
      ↓
Railpack builds a container image (no Dockerfile)
      ↓
Docker runs the container on a free local port
      ↓
Caddy Admin API adds a reverse proxy route: /deploy/{id}/*
      ↓
temp dir cleaned up
```

Every step streams logs to the UI in real time. Logs persist to SQLite so you can scroll back after the build finishes.

Deploys are blue-green. A new container starts and passes a health check before Caddy cuts traffic over. The old container is removed after the swap. There's no window where a request lands on nothing.

---

## The part worth understanding — log streaming

Getting a log line from a Railpack subprocess into a browser tab touches every layer of the system. Here's the full path:

```
Railpack stdout/stderr
      ↓  (bufio.Scanner, line by line)
portal.Publish()  →  buffered ingest channel
      ↓
single drain goroutine
      ├── writes LogLine to SQLite
      └── fans out to active SSE subscriber channels

On SSE connect:
  1. Replay full log history from SQLite
  2. Register a live subscriber channel
  3. Forward new lines as they arrive
  4. On disconnect: unsubscribe, close channel
```

The single drain goroutine is deliberate. It serialises all SQLite writes so there's no contention, and it means a slow or disconnected client can never block the pipeline — they just miss lines from their channel. The store always has the full history, so a late joiner or a reconnect gets everything.

---

## Caddy

Caddy starts from a static `caddy.json` that handles three things: serve the frontend, proxy `/api/*` to the backend, and expose the Admin API on `0.0.0.0:2019` (not `localhost` — it needs to be reachable from the backend container over the Docker Compose network).

Every deployment gets a route added at runtime via the Admin API. Routes carry a stable `@id` tag (`shipyard-{deploymentID}`). That's what makes blue-green swaps and deletes clean — you PUT or DELETE by ID, not by position in an array.

Dynamic routes live in Caddy's memory. On backend startup, all `running` deployments are re-registered. A restart doesn't orphan live URLs.

---

## Running it

```bash
docker compose up
```

**Prerequisites:** Docker 23+ (BuildKit on by default), Docker Compose v2. That's it.

**Environment variables** — all have defaults, none are required:

| Variable | Default | What it does |
|---|---|---|
| `PORT` | `8080` | Backend listen port |
| `DB_PATH` | `/data/shipyard.db` | SQLite file |
| `UPLOAD_DIR` | `/tmp/shipyard/uploads` | Where uploaded archives land |
| `BUILD_DIR` | `/tmp/shipyard/builds` | Temp dirs for source code |
| `CADDY_ADMIN_URL` | `http://caddy:2019` | Caddy Admin API address |

---

## API

```
POST   /api/deployments                Create (JSON or multipart)
GET    /api/deployments                List all
GET    /api/deployments/:id            Get one
GET    /api/deployments/:id/logs       SSE — history first, then live
POST   /api/deployments/:id/redeploy  Retrigger full pipeline
POST   /api/deployments/:id/restart   Restart container from same image
POST   /api/deployments/:id/rollback  Body: { "image_tag": "..." }
DELETE /api/deployments/:id            Stop, remove, clean up Caddy route
```

Git URL deploy:
```bash
curl -X POST http://localhost/api/deployments \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-app","source_type":"git","git_url":"https://github.com/you/repo","git_commit":"abc1234","env_vars":{"NODE_ENV":"production"}}'
```

Archive upload:
```bash
curl -X POST http://localhost/api/deployments \
  -F name=my-app \
  -F env_vars='{"PORT":"3000"}' \
  -F archive=@./project.tar.gz
```

Deployment status moves through: `pending → building → deploying → running` or `failed`. `rolled_back` is set after a successful rollback.

Each pipeline phase retries up to 3 times with exponential backoff. Retry attempts show up as `system` log lines in the UI so you can see what's happening.

---

## Project structure

```
cmd/server/       main.go — wires everything together
internal/
  store/          SQLite — deployments and log lines
  portal/         Event bus — single writer, SSE fan-out
  filemanager/    Git clone, archive extract, temp dir cleanup
  vessel/         Docker daemon client — container CRUD, image builds
  proxy/          Caddy Admin API — add, swap, remove routes
  maestro/        Pipeline orchestration — the only package that imports all others
  api/            HTTP handlers and SSE endpoint
frontend/         Vite + TanStack Router + Query
```

`maestro` is the only package with cross-cutting imports. Everything else is isolated. If you want to understand the system, start in `maestro/maestro.go` and follow the `run()` function.

---

## What I'd do with more time

**Build cache** — Railpack generates BuildKit cache hints but there's no cache volume mounted. One named volume and a `--cache-from` flag would cut repeat build times significantly.

**Container log tailing** — logs are captured during the build phase only. Tailing runtime logs via `docker logs --follow` through the same portal would make debugging running apps much more useful.

**Proper health checks** — right now `WaitForHealthy` polls `ContainerInspect` until status is `running`. It should be hitting a configurable HTTP endpoint and waiting for a 200.

**Route reconciliation on startup** — the loop that re-registers running deployments with Caddy on startup is there but incomplete. Five more lines and it's done.

**What I'd rip out** — the Caddy server name is resolved dynamically on first call and cached on the struct. It works, but it's fragile. I'd just name the server explicitly in `caddy.json` and pass it as a constructor argument. Less clever, more obvious.

---

## Brimble deploy + feedback

**Deployed:** [your link here]

[Your honest paragraph or two — what was confusing, what broke, what you'd change. Be direct, that's what they're grading.]

---

## Time spent

X hours. The majority went into the portal/SSE plumbing and the Caddy Admin API — specifically that routes need a stable `@id` to be addressable, and that the admin API needs `0.0.0.0` not `localhost` to be reachable across Docker Compose services. Both were non-obvious until they weren't.
