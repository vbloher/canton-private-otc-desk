CREATE TABLE rfqs
(
    id          UUID PRIMARY KEY,
    taker_party TEXT            NOT NULL,
    taker_hint  TEXT            NOT NULL,
    direction   TEXT            NOT NULL,
    qty         NUMERIC(38, 10) NOT NULL,
    base        TEXT            NOT NULL,
    quote       TEXT            NOT NULL,
    makers      JSONB           NOT NULL, -- [{"id": party id, "hint": party hint}]
    fee_bps     INT             NOT NULL,
    deadline    TIMESTAMPTZ     NOT NULL,
    status      TEXT            NOT NULL,
    created_at  TIMESTAMPTZ     NOT NULL,
    closed_at   TIMESTAMPTZ     NULL
);

CREATE INDEX rfqs_taker_idx ON rfqs (taker_party, created_at DESC);
CREATE INDEX rfqs_makers_idx ON rfqs USING GIN (makers jsonb_path_ops);
CREATE INDEX rfqs_open_deadline_idx ON rfqs (deadline) WHERE status = 'open';

CREATE TABLE quotes
(
    id          UUID PRIMARY KEY,
    rfq_id      UUID            NOT NULL REFERENCES rfqs (id),
    maker_party TEXT            NOT NULL,
    maker_hint  TEXT            NOT NULL,
    price       NUMERIC(38, 10) NOT NULL,
    valid_until TIMESTAMPTZ     NOT NULL,
    status      TEXT            NOT NULL,
    created_at  TIMESTAMPTZ     NOT NULL,
    opened_at   TIMESTAMPTZ     NULL, -- when the maker's allocation landed; a taker sees only opened quotes
    UNIQUE (rfq_id, maker_party)
);

CREATE INDEX quotes_status_idx ON quotes (status);

CREATE TABLE trades
(
    id          UUID PRIMARY KEY,
    rfq_id      UUID            NOT NULL REFERENCES rfqs (id),
    quote_id    UUID            NOT NULL UNIQUE REFERENCES quotes (id),
    taker_party TEXT            NOT NULL,
    taker_hint  TEXT            NOT NULL,
    maker_party TEXT            NOT NULL,
    maker_hint  TEXT            NOT NULL,
    direction   TEXT            NOT NULL,
    base        TEXT            NOT NULL,
    quote       TEXT            NOT NULL,
    qty         NUMERIC(38, 10) NOT NULL,
    price       NUMERIC(38, 10) NOT NULL,
    notional    NUMERIC(38, 10) NOT NULL,
    fee         NUMERIC(38, 10) NOT NULL,
    status      TEXT            NOT NULL,
    reason      TEXT            NOT NULL DEFAULT '',
    update_id   TEXT            NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ     NOT NULL,
    settled_at  TIMESTAMPTZ     NULL
);

CREATE INDEX trades_rfq_idx ON trades (rfq_id);
CREATE INDEX trades_taker_idx ON trades (taker_party, created_at DESC);
CREATE INDEX trades_maker_idx ON trades (maker_party, created_at DESC);
CREATE INDEX trades_settling_idx ON trades (created_at) WHERE status = 'settling';
