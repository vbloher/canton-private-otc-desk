ALTER TABLE quotes DROP CONSTRAINT quotes_rfq_id_maker_party_key;

CREATE UNIQUE INDEX quotes_rfq_maker_active_key ON quotes (rfq_id, maker_party)
    WHERE status NOT IN ('expired', 'cancelled', 'lost', 'failed');
