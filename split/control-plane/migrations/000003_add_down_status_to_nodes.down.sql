ALTER TABLE vpn_nodes DROP CONSTRAINT IF EXISTS vpn_nodes_status_check;

ALTER TABLE vpn_nodes
    ADD CONSTRAINT vpn_nodes_status_check
        CHECK (status IN ('active', 'inactive', 'draining', 'maintenance'));

UPDATE vpn_nodes
SET status = 'inactive'
WHERE status = 'down';
