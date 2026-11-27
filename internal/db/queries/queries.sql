-- name: UpsertOperator :exec
INSERT INTO operators (id, name) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING;

-- name: CreateStation :one
INSERT INTO stations (name, lat, lon, alt_m, min_el_deg, band, rate_per_min_minor)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: ListStations :many
SELECT * FROM stations ORDER BY name;

-- name: GetStation :one
SELECT * FROM stations WHERE id = $1;

-- name: GetStationByName :one
SELECT * FROM stations WHERE name = $1;

-- name: UpdateStation :one
UPDATE stations SET
    lat = COALESCE(sqlc.narg('lat'), lat),
    lon = COALESCE(sqlc.narg('lon'), lon),
    alt_m = COALESCE(sqlc.narg('alt_m'), alt_m),
    min_el_deg = COALESCE(sqlc.narg('min_el_deg'), min_el_deg),
    band = COALESCE(sqlc.narg('band'), band),
    rate_per_min_minor = COALESCE(sqlc.narg('rate_per_min_minor'), rate_per_min_minor)
WHERE id = sqlc.arg('id') RETURNING *;

-- name: DeleteStation :execrows
DELETE FROM stations WHERE id = $1;

-- name: CreateSatellite :one
INSERT INTO satellites (operator_id, name, norad_id, tle1, tle2) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListSatellites :many
SELECT * FROM satellites WHERE operator_id = $1 ORDER BY name;

-- name: GetSatellite :one
SELECT * FROM satellites WHERE id = $1 AND operator_id = $2;

-- name: UpsertPass :one
INSERT INTO passes (satellite_id, station_id, aos, tca, los, max_el, track)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (satellite_id, station_id, aos)
DO UPDATE SET tca = EXCLUDED.tca, los = EXCLUDED.los, max_el = EXCLUDED.max_el, track = EXCLUDED.track
RETURNING *;

-- name: GetPass :one
SELECT p.*, s.operator_id FROM passes p JOIN satellites s ON s.id = p.satellite_id WHERE p.id = $1;

-- name: LockStation :exec
SELECT pg_advisory_xact_lock(hashtext(@key::text));

-- name: ListStationWindows :many
SELECT p.aos, p.los FROM bookings b JOIN passes p ON p.id = b.pass_id WHERE p.station_id = $1;

-- name: ListPassesInRange :many
SELECT * FROM passes WHERE satellite_id = @satellite_id AND station_id = @station_id AND aos >= @from_t::timestamptz AND aos <= @to_t::timestamptz;

-- name: RevertDispatch :exec
UPDATE bookings SET status = 'booked', plan_sent_at = NULL WHERE id = $1;

-- name: CreateBooking :one
INSERT INTO bookings (pass_id, operator_id) VALUES ($1, $2) RETURNING id;

-- name: GetBooking :one
SELECT b.*, p.aos, p.los, p.station_id, p.satellite_id, st.name AS station_name, sa.name AS satellite_name
FROM bookings b JOIN passes p ON p.id = b.pass_id
JOIN stations st ON st.id = p.station_id JOIN satellites sa ON sa.id = p.satellite_id
WHERE b.id = $1;

-- name: ListBookings :many
SELECT b.*, p.aos, p.los, p.station_id, p.satellite_id, st.name AS station_name, sa.name AS satellite_name
FROM bookings b JOIN passes p ON p.id = b.pass_id
JOIN stations st ON st.id = p.station_id JOIN satellites sa ON sa.id = p.satellite_id
WHERE b.operator_id = $1 ORDER BY p.aos DESC;

-- name: DueBookings :many
SELECT b.id FROM bookings b JOIN passes p ON p.id = b.pass_id
WHERE b.status = 'booked' AND p.aos - make_interval(secs => @lead_secs::float8) <= @sim_now::timestamptz;

-- name: MarkDispatched :execrows
UPDATE bookings SET status = 'dispatched', plan_sent_at = now() WHERE id = $1 AND status = 'booked';

-- name: RecordFrame :exec
UPDATE bookings SET frames = frames + 1,
    locked_frames = locked_frames + @locked::int,
    lock_ratio = (locked_frames + @locked::int)::float8 / (frames + 1)
WHERE id = @id;

-- name: RecordAck :exec
UPDATE bookings SET ack_latency_ms = GREATEST(0, (extract(epoch FROM (sqlc.arg(ack_at)::timestamptz - plan_sent_at)) * 1000)::int)
WHERE id = @id AND plan_sent_at IS NOT NULL AND ack_latency_ms IS NULL;

-- name: RecordAOS :exec
UPDATE bookings SET status = 'tracking', aos_actual = @t::timestamptz WHERE id = @id;

-- name: RecordLOS :exec
UPDATE bookings SET los_actual = @t::timestamptz WHERE id = @id;

-- name: RecordDelivery :exec
UPDATE bookings SET bytes_delivered = @bytes::bigint, delivery_delay_ms = @delay_ms::int,
    status = CASE WHEN lock_ratio >= 0.8 THEN 'completed' ELSE 'failed' END
WHERE id = @id;

-- name: SlaByStation :many
SELECT st.name AS name, count(*)::int AS passes,
    (count(*) FILTER (WHERE b.lock_ratio >= 0.8))::int AS successes,
    COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY b.ack_latency_ms), 0)::float8 AS ack_p95_ms,
    COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY b.delivery_delay_ms), 0)::float8 AS delivery_p95_ms
FROM bookings b JOIN passes p ON p.id = b.pass_id JOIN stations st ON st.id = p.station_id
WHERE b.operator_id = $1 AND b.status IN ('completed', 'failed')
GROUP BY st.name ORDER BY st.name;

-- name: SlaBySatellite :many
SELECT sa.name AS name, count(*)::int AS passes,
    (count(*) FILTER (WHERE b.lock_ratio >= 0.8))::int AS successes,
    COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY b.ack_latency_ms), 0)::float8 AS ack_p95_ms,
    COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY b.delivery_delay_ms), 0)::float8 AS delivery_p95_ms
FROM bookings b JOIN passes p ON p.id = b.pass_id JOIN satellites sa ON sa.id = p.satellite_id
WHERE b.operator_id = $1 AND b.status IN ('completed', 'failed')
GROUP BY sa.name ORDER BY sa.name;

-- name: Billing :many
SELECT to_char(b.aos_actual, 'YYYY-MM')::text AS month, st.name AS station, st.rate_per_min_minor,
    (sum(extract(epoch FROM (b.los_actual - b.aos_actual))) / 60.0)::float8 AS minutes,
    round(sum(extract(epoch FROM (b.los_actual - b.aos_actual))) / 60.0 * st.rate_per_min_minor)::bigint AS amount_minor
FROM bookings b JOIN passes p ON p.id = b.pass_id JOIN stations st ON st.id = p.station_id
WHERE b.operator_id = $1 AND b.status IN ('completed', 'failed') AND b.aos_actual IS NOT NULL AND b.los_actual IS NOT NULL
GROUP BY 1, st.name, st.rate_per_min_minor ORDER BY 1, st.name;

-- name: GetSimClock :one
SELECT * FROM sim_clock WHERE id = 1;

-- name: SetSimClock :exec
UPDATE sim_clock SET real_ref = now(), sim_ref = @sim_ref::timestamptz, scale = @scale::float8 WHERE id = 1;
