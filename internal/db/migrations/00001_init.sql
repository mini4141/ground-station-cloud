-- +goose Up
CREATE TABLE operators (
    id   text PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE stations (
    id                 text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    name               text NOT NULL UNIQUE,
    lat                double precision NOT NULL,
    lon                double precision NOT NULL,
    alt_m              double precision NOT NULL DEFAULT 0,
    min_el_deg         double precision NOT NULL DEFAULT 5,
    band               text NOT NULL,
    rate_per_min_minor integer NOT NULL
);

CREATE TABLE satellites (
    id          text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    operator_id text NOT NULL REFERENCES operators (id),
    name        text NOT NULL,
    norad_id    integer NOT NULL,
    tle1        text NOT NULL,
    tle2        text NOT NULL
);

CREATE TABLE passes (
    id           text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    satellite_id text NOT NULL REFERENCES satellites (id) ON DELETE CASCADE,
    station_id   text NOT NULL REFERENCES stations (id) ON DELETE CASCADE,
    aos          timestamptz NOT NULL,
    tca          timestamptz NOT NULL,
    los          timestamptz NOT NULL,
    max_el       double precision NOT NULL,
    track        jsonb NOT NULL,
    UNIQUE (satellite_id, station_id, aos)
);

CREATE TABLE bookings (
    id                text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    pass_id           text NOT NULL UNIQUE REFERENCES passes (id) ON DELETE CASCADE,
    operator_id       text NOT NULL REFERENCES operators (id),
    status            text NOT NULL DEFAULT 'booked',
    created_at        timestamptz NOT NULL DEFAULT now(),
    plan_sent_at      timestamptz,
    ack_latency_ms    integer,
    aos_actual        timestamptz,
    los_actual        timestamptz,
    delivery_delay_ms integer,
    frames            integer NOT NULL DEFAULT 0,
    locked_frames     integer NOT NULL DEFAULT 0,
    lock_ratio        double precision NOT NULL DEFAULT 0,
    bytes_delivered   bigint NOT NULL DEFAULT 0
);

-- single-row simulated clock shared by API, scheduler and Lambdas
CREATE TABLE sim_clock (
    id       integer PRIMARY KEY CHECK (id = 1),
    real_ref timestamptz NOT NULL,
    sim_ref  timestamptz NOT NULL,
    scale    double precision NOT NULL
);
INSERT INTO sim_clock (id, real_ref, sim_ref, scale) VALUES (1, now(), '2026-11-27 09:30:00+00', 20);

-- +goose Down
DROP TABLE sim_clock, bookings, passes, satellites, stations, operators;
