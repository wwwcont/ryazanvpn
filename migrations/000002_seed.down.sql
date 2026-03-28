DELETE FROM vpn_nodes
WHERE name IN ('mvp-node-1', 'mvp-node-2');

DELETE FROM invite_codes
WHERE code IN ('1111', '2222', '3333');
