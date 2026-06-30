# Piped

Submit a Git URL or a project archive. Piped builds it into a container image, runs it, and gives you a live URL — all from a single page.

```bash
git clone https://github.com/Richd0tcom/piped
cd piped
docker compose up
```

Open `http://localhost`. No accounts. No config. Done.

Sample git url to test deployment: https://github.com/Richd0tcom/login-form

---


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


