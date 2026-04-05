CREATE TABLE IF NOT EXISTS node_throughput_samples (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vpn_node_id UUID NOT NULL REFERENCES vpn_nodes (id) ON DELETE CASCADE,
    captured_at TIMESTAMPTZ NOT NULL,
    window_sec BIGINT NOT NULL DEFAULT 60,
    rx_bytes BIGINT NOT NULL DEFAULT 0,
    tx_bytes BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    throughput_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
    peers_total INTEGER NOT NULL DEFAULT 0,
    peers_resolved INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_node_throughput_samples_node_captured UNIQUE (vpn_node_id, captured_at)
);

CREATE INDEX IF NOT EXISTS idx_node_throughput_samples_node_captured
    ON node_throughput_samples (vpn_node_id, captured_at DESC);

CREATE TRIGGER trg_node_throughput_samples_updated_at
BEFORE UPDATE ON node_throughput_samples
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
