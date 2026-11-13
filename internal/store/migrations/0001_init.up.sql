-- Hosts reporting through an agent. hostname is the stable identity.
CREATE TABLE hosts (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text NOT NULL,
    hostname         text NOT NULL UNIQUE,
    os               text NOT NULL DEFAULT '',
    platform         text NOT NULL DEFAULT '',
    platform_version text NOT NULL DEFAULT '',
    kernel_version   text NOT NULL DEFAULT '',
    arch             text NOT NULL DEFAULT '',
    cpu_count        integer NOT NULL DEFAULT 0,
    agent_version    text NOT NULL DEFAULT '',
    boot_time        timestamptz,
    report_interval_s integer NOT NULL DEFAULT 15,
    last_seen_at     timestamptz NOT NULL DEFAULT now(),
    created_at       timestamptz NOT NULL DEFAULT now()
);

-- Time series of host resource samples.
CREATE TABLE host_metrics (
    host_id    uuid NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    ts         timestamptz NOT NULL,
    cpu_pct    double precision NOT NULL,
    mem_pct    double precision NOT NULL,
    mem_used   bigint NOT NULL,
    mem_total  bigint NOT NULL,
    swap_pct   double precision NOT NULL DEFAULT 0,
    disk_pct   double precision NOT NULL,
    disk_used  bigint NOT NULL,
    disk_total bigint NOT NULL,
    net_rx_bps double precision NOT NULL,
    net_tx_bps double precision NOT NULL,
    load1      double precision NOT NULL DEFAULT 0,
    load5      double precision NOT NULL DEFAULT 0,
    load15     double precision NOT NULL DEFAULT 0,
    uptime_s   bigint NOT NULL,
    proc_count integer NOT NULL,
    PRIMARY KEY (host_id, ts)
);
CREATE INDEX host_metrics_ts_idx ON host_metrics (ts);

-- Latest detailed state per host (disks, interfaces, processes).
CREATE TABLE host_snapshots (
    host_id      uuid PRIMARY KEY REFERENCES hosts(id) ON DELETE CASCADE,
    collected_at timestamptz NOT NULL,
    disks        jsonb NOT NULL DEFAULT '[]',
    interfaces   jsonb NOT NULL DEFAULT '[]',
    processes    jsonb NOT NULL DEFAULT '[]'
);

-- HTTP monitors.
CREATE TABLE monitors (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL,
    url             text NOT NULL,
    method          text NOT NULL DEFAULT 'GET',
    interval_s      integer NOT NULL CHECK (interval_s BETWEEN 10 AND 86400),
    timeout_ms      integer NOT NULL CHECK (timeout_ms BETWEEN 100 AND 60000),
    expected_status integer NOT NULL DEFAULT 0,
    headers         jsonb NOT NULL DEFAULT '{}',
    enabled         boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE check_results (
    id          bigserial PRIMARY KEY,
    monitor_id  uuid NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    checked_at  timestamptz NOT NULL,
    success     boolean NOT NULL,
    status_code integer NOT NULL DEFAULT 0,
    response_ms integer NOT NULL,
    error       text NOT NULL DEFAULT ''
);
CREATE INDEX check_results_monitor_ts_idx ON check_results (monitor_id, checked_at DESC);
CREATE INDEX check_results_ts_idx ON check_results (checked_at);

CREATE TABLE alert_rules (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('host_metric','host_offline','monitor_down','monitor_latency')),
    metric     text NOT NULL DEFAULT '',
    operator   text NOT NULL DEFAULT '>' CHECK (operator IN ('>','>=','<','<=')),
    threshold  double precision NOT NULL DEFAULT 0,
    for_count  integer NOT NULL DEFAULT 1 CHECK (for_count BETWEEN 1 AND 100),
    severity   text NOT NULL CHECK (severity IN ('critical','warning','info')),
    host_id    uuid REFERENCES hosts(id) ON DELETE CASCADE,
    monitor_id uuid REFERENCES monitors(id) ON DELETE CASCADE,
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE alerts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id         uuid NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    target_type     text NOT NULL CHECK (target_type IN ('host','monitor')),
    target_id       uuid NOT NULL,
    target_name     text NOT NULL,
    state           text NOT NULL CHECK (state IN ('firing','acknowledged','resolved')),
    severity        text NOT NULL,
    message         text NOT NULL,
    value           double precision NOT NULL DEFAULT 0,
    triggered_at    timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    resolved_at     timestamptz
);
-- At most one open (non-resolved) alert per rule and target. This is the
-- deduplication guarantee: concurrent evaluators cannot open duplicates.
CREATE UNIQUE INDEX alerts_open_unique ON alerts (rule_id, target_id) WHERE state <> 'resolved';
CREATE INDEX alerts_triggered_idx ON alerts (triggered_at DESC);
CREATE INDEX alerts_target_idx ON alerts (target_id) WHERE state <> 'resolved';

CREATE TABLE webhooks (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    url        text NOT NULL,
    secret     text NOT NULL DEFAULT '',
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE events (
    id          bigserial PRIMARY KEY,
    ts          timestamptz NOT NULL DEFAULT now(),
    kind        text NOT NULL,
    severity    text NOT NULL DEFAULT 'info',
    message     text NOT NULL,
    target_type text NOT NULL DEFAULT '',
    target_id   text NOT NULL DEFAULT ''
);
CREATE INDEX events_ts_idx ON events (ts DESC);

-- Sensible default rules so a fresh install alerts out of the box.
INSERT INTO alert_rules (name, kind, metric, operator, threshold, for_count, severity) VALUES
    ('High CPU usage',      'host_metric',     'cpu_pct',  '>', 90,   4, 'warning'),
    ('High memory usage',   'host_metric',     'mem_pct',  '>', 90,   4, 'warning'),
    ('Disk almost full',    'host_metric',     'disk_pct', '>', 90,   1, 'critical'),
    ('Host offline',        'host_offline',    '',         '>', 90,   1, 'critical'),
    ('Endpoint down',       'monitor_down',    '',         '>', 0,    3, 'critical'),
    ('Slow endpoint',       'monitor_latency', '',         '>', 2000, 3, 'warning');
