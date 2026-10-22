# Daml assets

Nothing here settles a trade of ours: the desk settles through the token registry's own
`SettlementFactory_SettleBatch` (Canton token standard v2, CIP-112).

- `deps/` — the token-standard interface DARs (v1 and v2) and utils, vendored as
  `data-dependencies` from Splice 0.6.12 (v1) and 0.7.5 (v2), Apache-2.0. Their package ids
  match DevNet/MainNet.
- `test-token-v2/` — upstream `splice-test-token-v2` (Splice tag 0.7.5, Apache-2.0, no local
  edits): the sandbox token rail. On a real network a token registry plays this role.
- `setup/` — the sandbox seed (`Setup:setup`): parties `operator`, `makerA`, `makerB`, one
  `TokenRules` registry and the house makers' holdings.

Build with DPM / Daml SDK 3.5.2: `dpm build --all` (run by `scripts/ledger-start.sh`).

`deps/` and `test-token-v2/` are third-party code under the Apache License 2.0, a copy of which
is in [`LICENSE-APACHE-2.0`](LICENSE-APACHE-2.0); `setup/` is part of this project (GPL-3.0).
