ALTER TABLE client_addresses
ADD CONSTRAINT unique_client_address_type
UNIQUE (client_id, type);
