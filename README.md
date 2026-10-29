# Private OTC Desk for Canton

[![CI](https://github.com/vbloher/canton-private-otc-desk/actions/workflows/ci.yml/badge.svg)](https://github.com/vbloher/canton-private-otc-desk/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/go-1.27-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-GPL--3.0-blue)

A confidential request-for-quote desk for large token swaps on [Canton Network](https://www.canton.network).
A taker asks the makers they choose for a price, accepts the best quote, and the swap settles
atomically (delivery versus payment) in a single Canton transaction. Nobody outside the trade
sees the price or the counterparty.

Wallets are self-custodial: a user's Ed25519 party key is generated in the browser, wrapped
with a passkey (WebAuthn PRF) and never leaves it. Every ledger action of a user is an
interactive submission — the desk prepares it, the browser signs it, the desk executes it.

This repository is a deliberately small open-source implementation: Go microservices built on
[Clean Architecture](https://github.com/vbloher/go-clean-architecture-template), a layered React
SPA, and a local Canton sandbox with the token-standard test rail.

## Features

- **Passkey wallet** — create a wallet (an external Canton party) with a passkey; connect it
  again later with the same passkey. No passwords, no e-mail.
- **RFQ trading** — buy or sell any instrument pair, ask several makers at once, see quotes
  appear live, accept the best one.
- **Atomic settlement** — a quote is the maker's committed token-standard v2 allocation; the
  accept adds the taker's; the desk settles every leg in one `SettlementFactory_SettleBatch`
  transaction. All or nothing.
- **Maker mode** — any wallet can become a maker and quote from the browser; bots quote over
  the same API with an API key. A demo maker bot quotes for the two house makers.
- **Wallet** — balances (available and locked), test-token faucet, deposits (accept incoming
  transfers) and withdrawals to any Canton party.
- **Live updates** — a WebSocket pushes RFQ, quote, trade and signing events.

## Architecture

```mermaid
graph LR
  B[Browser SPA] -->|HTTP + WS| W[web · nginx]
  BOT[maker-bot] -->|X-API-Key| G
  W -->|/api| G[gateway]
  G -->|HTTP| I[identity]
  G -->|HTTP| T[trading]
  G -->|HTTP| L[ledger]
  G -->|gRPC auth| I
  T -->|gRPC| L
  T -->|gRPC| I
  I -->|gRPC| L
  L -->|JSON Ledger API v2| C[(Canton sandbox)]
  I --- PG[(Postgres)]
  T --- PG
  L --- PG
  I --- R[(Redis)]
  T --- R
  L --- R
  G --- R
```

| Service | Responsibility | Ports |
|---|---|---|
| `gateway` | single public origin: authenticates the session cookie or API key, forwards the caller to the services, hosts the WebSocket fed by Redis pub/sub | HTTP 8080 |
| `identity` | passkey wallets, sessions, challenges, house makers, maker mode | HTTP 8081 · gRPC 9081 |
| `trading` | RFQs, quotes, trades and the settlement worker | HTTP 8082 |
| `ledger` | everything Canton: parties, holdings, transfers, allocations, settlement, the sign queue | HTTP 8083 · gRPC 9083 |
| `maker-bot` | demo market maker for `makerA` and `makerB` | — |
| `web` | the SPA behind nginx, which also proxies `/api` | 3000 |

Each service follows the template's layout — `cmd/app`, `config`, `internal/{app,domain,dto,
usecase,adapter,controller}`, `migration` — with ports declared by the use cases and implemented
by adapters. Postgres holds one database per service; Redis is the cache (sessions, desk facts,
the registry contract), the store for one-shot challenges and the event bus. HTTP contracts are
OpenAPI (`api/http`, servers and clients generated with oapi-codegen); internal contracts are
protobuf (`api/grpc`).

### How a trade settles

1. The taker creates an RFQ (`BUY 0.5 CBTC` against `USDCx`) for chosen makers.
2. A maker quotes a price. The quote is the maker's **committed allocation** on the ledger, so
   a quote is always funded; the taker sees it once it lands.
3. The taker accepts. The taker (and, for the fee, the desk) allocate their sides.
4. The desk, the settlement's only executor, settles every leg — base, quote notional and the
   desk fee — in one transaction. Losing quotes are cancelled and their funds released.

The fee (`FEE_BPS`, default 10) is paid by the maker in the quote asset. Amounts are exact
decimals with ten fraction digits, floored like Daml's `Numeric 10`.

## Quick start

Requirements: Docker, [DPM](https://docs.digitalasset.com) with Daml SDK 3.5.2
(`curl https://get.digitalasset.com/install/install.sh | sh`, then `dpm install 3.5.2`, Java 21),
and a browser with passkeys that support the PRF extension (Chrome, Edge, Safari 18+).

```bash
make ledger    # terminal 1: Canton sandbox + test tokens + demo parties (keep it running)
make up-all    # terminal 2: Postgres, Redis, the services, the maker bot and the web app
```

`make up-all` works on macOS (Docker Desktop, OrbStack) and Linux: on Linux it adds
[`docker-compose.host.yml`](docker-compose.host.yml), which puts the app containers on the host
network — bound to 127.0.0.1 — so they can reach the sandbox on the host's loopback. Either way
nothing listens beyond 127.0.0.1: the web app (:3000), the gateway (:8080), Postgres (:5432) and
Redis (:6379) — and on Linux also the services' own ports (8081–8083, 9081, 9083), which any
local process can reach, so run it on a single-user machine.

The sandbox keeps its ledger in memory. If you restart `make ledger`, the wallets in the
database point at parties that no longer exist: run `make clean` (drops the databases) and
`make up-all` again.

Open <http://localhost:3000>:

1. **Create wallet** — pick a name and confirm the passkey prompt.
2. **Wallet → Get test tokens** — 1 CBTC, 100 000 USDCx and 10 CETH arrive once the
   auto-signer accepts the offers.
3. **Request quotes** — e.g. buy 0.1 CBTC for USDCx from `makerA` and `makerB`. The maker bot
   answers within a second.
4. **Accept** the best quote and watch the trade settle; balances move in one transaction.
5. Optional: turn **maker mode** on in a second browser profile and quote from the UI.

## Development

Requirements: Go 1.27.3, Node 24 with `corepack enable` (pnpm's version is pinned in
`web/package.json`), plus the quick-start requirements.

```bash
make bin-deps            # protoc and golangci-lint into ./bin
make ledger              # Canton sandbox
make up                  # Postgres and Redis only (127.0.0.1)
make run-ledger          # each in its own terminal: run-ledger, run-identity, run-trading,
make run-identity        # run-gateway, run-maker-bot (each creates services/<name>/.env from the example)
make web-dev             # Vite on :3000, proxying /api to the gateway on :8080
```

| Command | Description |
|---|---|
| `make test` | Go unit tests (race detector, shuffled like CI) |
| `make integration-test` | service tests against real Postgres and Redis (`make up`) |
| `make sandbox-test` | ledger adapter tests against the running sandbox |
| `make e2e-test` | black-box end-to-end suite against the running stack |
| `make lint` | golangci-lint |
| `make generate` | regenerate protobuf, OpenAPI servers/clients and mocks |
| `make vuln` | govulncheck (pinned as a Go tool) |
| `make clean` | stop the stack and drop its databases |
| `pnpm --dir web test` | web unit tests (`lint`, `typecheck`, `build`, `gen:api` likewise) |

To change an HTTP API, edit `api/http/<name>_v1.yaml` and run `make generate` (and
`pnpm --dir web gen:api`); the compiler then points at every handler that no longer matches.

## API

Everything is served by the gateway under `/api/v1`. Browsers authenticate with the
`otc_session` cookie; bots send `X-API-Key`. The sandbox's house makers have the keys
`demo-key-a` (`makerA`) and `demo-key-b` (`makerB`).

```bash
# a bot looks for RFQs it was asked to quote and answers one
curl -s -H 'X-API-Key: demo-key-a' localhost:8080/api/v1/rfqs/incoming
curl -s -H 'X-API-Key: demo-key-a' -H 'content-type: application/json' \
  -d '{"price":"100150.0","ttlSeconds":60}' localhost:8080/api/v1/rfqs/<rfqId>/quotes
```

| Area | Contract |
|---|---|
| wallets, sessions, makers | [`api/http/identity_v1.yaml`](api/http/identity_v1.yaml) |
| RFQs, quotes, trades | [`api/http/trading_v1.yaml`](api/http/trading_v1.yaml) |
| balances, transfers, signing | [`api/http/wallet_v1.yaml`](api/http/wallet_v1.yaml) |
| WebSocket `/api/v1/ws` | `{"type": "rfq.created" \| "rfq.updated" \| "quote.updated" \| "trade.updated" \| "sign.requested" \| "sign.completed" \| "wallet.updated", "data": {...}}` |

## Configuration

Every service reads environment variables (and `.env` in its directory when present).
Common: `HTTP_HOST` (empty = every interface) and `HTTP_PORT`, `LOGGER_LEVEL` (info),
`LOGGER_PRETTY_CONSOLE`, `OTEL_ENDPOINT`, `OTEL_SERVICE_NAME`,
`POSTGRES_HOST/PORT/USER/PASSWORD/DB_NAME`, `REDIS_ADDR`; services with gRPC also take
`GRPC_HOST` and `GRPC_PORT`. The web image takes `NGINX_HOST`, `NGINX_PORT` (80) and
`GATEWAY_UPSTREAM` (gateway:8080).

| Service | Variables (defaults) |
|---|---|
| identity | `GRPC_PORT` (9081), `LEDGER_GRPC_ADDR` (localhost:9083), `HOUSE_MAKERS` (makerA:demo-key-a,makerB:demo-key-b), `SESSION_TTL` (168h), `COOKIE_SECURE` (false) |
| ledger | `GRPC_PORT` (9083), `CANTON_URL` (http://localhost:6864), `CANTON_USER_ID` (participant_admin), `OPERATOR_HINT` (operator), `HOUSE_PARTIES` (makerA,makerB), `INSTRUMENTS` (CBTC:Bitcoin:8,USDCx:USD Coin:2,CETH:Ether:6), `FAUCET_AMOUNTS` (CBTC:1,USDCx:100000,CETH:10), `ACTION_TTL` (10m) |
| trading | `LEDGER_GRPC_ADDR` (localhost:9083), `IDENTITY_GRPC_ADDR` (localhost:9081), `FEE_BPS` (10), `WORKER_INTERVAL` (1s), `SETTLE_TIMEOUT` (10m) |
| gateway | `IDENTITY_GRPC_ADDR` (localhost:9081), `IDENTITY_URL`, `TRADING_URL`, `LEDGER_URL` (localhost:8081–8083) |
| maker-bot | `GATEWAY_URL` (http://localhost:8080), `MAKER_KEYS` (demo-key-a:15,demo-key-b:40 — key:spread bps), `PRICES_USD` (CBTC:100000,CETH:4000,USDCx:1), `QUOTE_TTL` (60s), `POLL_INTERVAL` (1s) |

## Deployment

Two environments, each on a VPS that runs the whole stack with Docker Compose:

| Trigger | Images (`ghcr.io/<owner>/<repo>/<image>`) | Environment |
|---|---|---|
| push to `dev` | `dev`, `sha-<short>` | `dev` |
| tag `v1.2.3` | `1.2.3`, `1.2`, `latest`, `sha-<short>` | `prod` |

[`release.yml`](.github/workflows/release.yml) runs CI, publishes the six images (multi-arch, public
like the repository) and deploys over SSH: it copies the rendered
[`deploy/docker-compose.yml`](deploy/docker-compose.yml), `postgres/init.sql` and `.env.example` to
`~/otc-desk-<environment>/` on the server, then runs `docker compose pull` and
`docker compose up -d --wait`. The job fails when a container does not come up healthy.

Only the web app is published, on `127.0.0.1:${WEB_PORT}`; its nginx serves the SPA and proxies
`/api/` and the WebSocket to the gateway. A reverse proxy on the host terminates TLS for the
domain. HTTPS is required: passkeys need a secure origin and the session cookie is `Secure`.

### Server

Once per server:

1. Install Docker Engine with the Compose plugin and add the SSH user to the `docker` group.
2. Install [Caddy](https://caddyserver.com/docs/install) and point the domain at it; Caddy obtains
   the certificate and passes WebSocket upgrades on its own:

   ```caddyfile
   desk.example.com {
       reverse_proxy 127.0.0.1:3000
   }
   ```

   With both environments on one server, give each its own `WEB_PORT` and site block.

### GitHub

Create the environments `dev` and `prod` (**Settings → Environments**; required reviewers on `prod`
are a good idea) with these secrets:

| Secret | Value |
|---|---|
| `SSH_HOST` | server address |
| `SSH_USER` | user that runs `docker` |
| `SSH_KEY` | private key of a key pair whose public half is in that user's `authorized_keys` |
| `SSH_KNOWN_HOSTS` | output of `ssh-keyscan <host>` |
| `SSH_PORT` | optional, 22 by default |

Then create the `dev` branch. The deployment's settings are not in GitHub.

### First deploy

The first run copies the files and stops because `.env` is missing. On the server:

```bash
cd ~/otc-desk-dev
cp .env.example .env
$EDITOR .env
```

Fill in at least `POSTGRES_PASSWORD`, `CANTON_URL`, `HOUSE_MAKERS` and `MAKER_KEYS`, then re-run the
job (**Actions → the run → Re-run failed jobs**). The deploy job never writes `.env`; when a release
adds a setting, it shows up in `.env.example` next to it. Local changes to the stack go into
`docker-compose.override.yml` in the same directory, which Compose reads and the job leaves alone;
`docker-compose.yml` itself is replaced on every deploy.

### Rollback

Run the workflow by hand on the tag to go back to (**Actions → Release → Run workflow**, pick the
tag): it tests, rebuilds from cache and deploys that version. On the server
`docker compose up -d` in the directory always starts the version deployed last.

## Repository layout

```
api/            OpenAPI and protobuf contracts
gen/            generated servers, clients and gRPC stubs
pkg/            shared infrastructure (logger, metrics, otel, postgres, redis, transactions,
                decimal money math, events, principal headers, gRPC helpers, migrations)
services/       gateway, identity, trading, ledger, maker-bot
web/            React SPA: domain / application / infrastructure / presentation
daml/           vendored token-standard DARs, the sandbox test token, the demo seed
deploy/         the stack for a VPS (docker-compose.yml, .env.example), Postgres init script
scripts/        sandbox start script
test/e2e/       end-to-end suite
docs/           design and plan
```

## Limits

This is a reference implementation for a local sandbox, kept small on purpose:

- One target: a Canton sandbox, where a `TokenRules` contract plays the token registry.
  Real networks need OAuth for the ledger and the registries' HTTP APIs, so a deployment
  (`CANTON_URL`) still needs a participant set up like the sandbox.
- It is not a hardened deployment: locally the house makers' API keys are public defaults,
  anyone signed in can mint test tokens, and identity, trading and ledger trust the caller
  headers the gateway sets — never expose them (or the sandbox) beyond the compose network.
- The browser signs prepared-transaction hashes without decoding them; a production wallet
  should show the user what it signs. A prepared transaction waits for its signature for up to
  `ACTION_TTL` (10 min); the sandbox accepted signatures 90 s after preparation in testing, but
  real networks may enforce a shorter window.
- Losing your passkey loses the wallet: there is no backup phrase or recovery.
- No invites, rate limiting, e-mail or KYC.

## License

[GPL-3.0](LICENSE). `daml/deps` and `daml/test-token-v2` are vendored from
[Splice](https://github.com/hyperledger-labs/splice) under Apache-2.0.
