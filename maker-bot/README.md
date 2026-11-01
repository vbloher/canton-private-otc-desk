# OTC Desk maker bot

A demo market maker for the [Private OTC Desk](../README.md) on Canton Network.

It is a separate application that works with the desk only through the public API, the same
`/api/v1` HTTP routes and `/api/v1/ws` WebSocket that any third-party maker uses. It holds its
own Ed25519 key: the desk only ever sees the public half and never signs anything for the maker.
Nothing in this directory imports the desk's code.

Fork it, change the pricing, run your own maker.

## What it does, forever

1. **Registers itself** on the first run. It generates an Ed25519 key, registers a key account
   through `/api/v1/auth/register/start`, `/party` and `/complete`, signs the onboarding hash
   and keeps the key in `state/identity.json` (mode 0600). Later runs sign in by signing a
   one-time challenge with that key and get a session; a session that runs out is renewed on
   the next `401`.
2. **Stays online.** It turns maker mode on and keeps one WebSocket open. Takers only see and
   invite makers that are connected right now. The bot pings the desk, reconnects 5 s after a
   lost connection, and re-reads `/rfqs/incoming` on every connect, so a reconnect is a resync.
3. **Quotes every RFQ it can settle** at the mid from its USD price table, moved by
   `SPREAD_BPS` in its favour. It skips an RFQ it cannot price, one that closes in less than
   10 s, and one that would commit more than `MAX_SPEND_SHARE` of a free balance. A quote that
   expires, or whose trade fails, is quoted again while the RFQ is open; a quote whose own
   allocation failed waits for the next reconnect.
4. **Signs what it is handed**: the token-standard allocations that are its quotes and the
   accepts of incoming transfers. It never signs a `transfer-out`, so it never withdraws.
5. **Keeps itself funded**: it accepts every incoming transfer and, where the desk runs a
   faucet (DevNet), draws from it when a balance drops under ten drips — at most once an hour,
   so a token the faucet cannot send is not asked for again and again.

## Run

Against a desk on `http://localhost:8080`:

```sh
go run ./cmd/maker-bot
```

Against another desk, with an `.env` file (every variable is optional):

```sh
cp .env.example .env
go run ./cmd/maker-bot
```

With Docker, keeping the identity in a named volume:

```sh
docker compose up -d --build
docker compose logs -f
```

The first run registers a new party and logs its id and hint. Registration and the first
faucet drip both settle on the ledger, so give it a minute.

## Configuration

| Variable          | Default                       | Meaning                                                     |
| ----------------- | ----------------------------- | ----------------------------------------------------------- |
| `DESK_URL`        | `http://localhost:8080`       | The desk's base URL; the API lives under `/api/v1`          |
| `MAKER_NAME`      | `OTC Demo Maker`              | Display name at registration, 2–32 characters; the hint derives from it |
| `SPREAD_BPS`      | `15`                          | Half-spread against the mid, in basis points                |
| `PRICES_USD`      | `CBTC:100000,USDCx:1,CC:0.15` | USD price per symbol; the mid of a pair is base over quote  |
| `QUOTE_TTL`       | `600s`                        | How long a quote stands (10 s–600 s), never past the RFQ deadline |
| `MAX_SPEND_SHARE` | `0.5`                         | Largest share of a free balance one quote may commit        |
| `POLL_INTERVAL`   | `10s`                         | How often it signs leftovers, accepts transfers and checks the faucet |
| `STATE_FILE`      | `state/identity.json`         | The identity file; `/data/identity.json` in the image       |
| `LOG_LEVEL`       | `info`                        | `debug`, `info`, `warn` or `error`; logs are JSON on stdout |

## Making it yours

| Package             | What lives there                                                              |
| ------------------- | ----------------------------------------------------------------------------- |
| `internal/strategy` | **The strategy**: the mid, the quoted price and what a trade costs the bot. Start here. |
| `internal/bot`      | The loop: what to quote and when, signing, housekeeping, registration.        |
| `internal/desk`     | The client for the slice of the public API the bot uses, with its session.    |
| `internal/deskapi`  | Clients generated from the desk's OpenAPI specs (`go generate ./...`).        |
| `internal/stream`   | The WebSocket: events, pings, reconnects.                                     |
| `internal/identity` | The key file.                                                                 |
| `internal/config`   | Environment to typed config; add your knobs here and validate them here.      |

Whatever you change, keep the key on your side: sign with it, never send it.

Checks:

```sh
go test -race ./...
golangci-lint run ./...
```

## The identity file

The file is the party. Losing it loses the party: a new run registers a fresh maker and
whatever the old party held stays with it. A run that crashed halfway through registration
resumes with the same key. If the file is unreadable, or the desk does not know its party, the
bot stops and says so instead of registering a new party over it.

## Networks

The bot blocks no network. On MainNet there is no faucet: fund the party with a transfer, and
the bot accepts it.

## Disclaimer

The default strategy is naive: static mids plus a spread, with no inventory, hedging or
adverse-selection management. Replace it before the bot trades real money.

## License

MIT — see [LICENSE](LICENSE). The rest of the repository is GPL-3.0.
