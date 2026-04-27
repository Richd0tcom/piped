# Piped (that one brimble assessment)

Submit a Git URL or a project archive. Piped builds it into a container image, runs it, and gives you a live URL — all from a single page.

```bash
git clone https://github.com/Richd0tcom/that-one-brimble-assessment
cd that-one-brimble-assessment
docker compose up
```

Open `http://localhost`. No accounts. No config. Done.

Sample git url to test deployment: https://github.com/Richd0tcom/login-form

---

## Why Go

The brief preferred TypeScript but personally and professionallly these are the types of apps you build with Go. The backend's entire job is running subprocesses (Railpack), piping their output line by line, managing containers.  Go does this well and handles it without ceremony. Would using TS have cut down on some of my pain and suffering? probably. But I have so much control and visibility with Go that I can see exactly what's happening at every step. 


---

## How the pipeline works

```
Git URL or archive
      ↓
clone / extract into a temp dir
      ↓
Railpack builds a container image (no Dockerfile)
      ↓
Docker runs the container
      ↓
Caddy Admin API adds a reverse proxy route: /deploy/{id}/*
      ↓
temp dir cleaned up
```

Every step streams logs to the UI in real time. Logs persist to SQLite so you can scroll back after the build finishes.



---

## The parts I think you should see - log streaming

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

Every deployment gets a route added at runtime via the Admin API. Routes carry a stable `@id` tag (`piped-{deploymentID}`). This makes for clean blue-green swaps and deletes .

Dynamic routes live in Caddy's memory. On backend startup, all `running` deployments are re-registered. A restart doesn't orphan live URLs.

---

## Running it

```bash
docker compose up
```

**Prerequisites:** Docker 23+ (BuildKit on by default), Docker Compose v2. That's it.

**Environment variables** — all have defaults, none are required fom you:


Deployment status moves through: `pending → building → deploying → running` or `failed`.
 <!-- `rolled_back` is set after a successful rollback. -->

Each pipeline phase retries up to 3 times with exponential backoff. Retry attempts show up as `system` log lines in the UI so you can see what's happening.

---

## Project structure

```
cmd/main.go — wires everything together
      internal/
            store/          SQLite — deployments and log lines storage
            portal/         Event bus — single writer, SSE fan-out
            filemanager/    Git clone, archive extract, temp dir cleanup
            vessel/         Docker daemon client — container CRUD, image builds
            proxy/          Caddy Admin API — add, swap, remove routes
            maestro/        Pipeline orchestration — the only package that imports all others
core/            HTTP handlers and routing

ui/         Vite + TanStack Router + Query
```

`maestro` is the only package with cross-cutting imports. Everything else is isolated. If you want to understand the system, start in `maestro/maestro.go` and follow the `run()` function.

---

## What I'd do with more time

**Build cache** — Railpack generates BuildKit cache hints but there's no cache volume mounted. One named volume and a `--cache-from` flag would cut repeat build times significantly.

**Container log tailing** — logs are captured during the build phase only. Tailing runtime logs via `docker logs --follow` through the same portal would make debugging running apps much more useful.

**Proper health checks** — right now `WaitForHealthy` polls `ContainerInspect` until status is `running`. It should be hitting a configurable HTTP endpoint and waiting for a 200.

**Route reconciliation on startup** — the loop that re-registers running deployments with Caddy on startup is there but incomplete. Five more lines and it's done.



## Time spent

This one took a while. roughly 19 hours (wakatime sum) and even then it still feels largely unfinished. The majority went into the portal/SSE plumbing and the Caddy Admin configuration as well as Container orchestration and configuration. would have been faster if I had just prompted my way from the start. 
