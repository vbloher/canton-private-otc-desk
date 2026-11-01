DROP INDEX quotes_rfq_maker_active_key;

ALTER TABLE quotes ADD CONSTRAINT quotes_rfq_id_maker_party_key UNIQUE (rfq_id, maker_party);
