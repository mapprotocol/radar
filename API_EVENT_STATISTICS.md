# Daily Event Statistics

`GET /v1/event/statistics` returns daily event counts for all known chains. It
uses the same API key and IP whitelist authentication as other `/v1` endpoints.
No query parameters are required.

```bash
curl 'http://localhost:8080/v1/event/statistics' \
  -H 'X-API-Key: <api-key>'
```

## Counting Rules

- `message_out`: `mos.project_id = 1`, with an associated `event.format` whose
  event name is `MessageOut`.
- `message_in`: `mos.project_id = 2`, with an associated `event.format` whose
  event name is `MessageIn`.
- The join uses both `event_id` and `project_id`. Different ABI signatures of
  the same event name are included. Names are case-sensitive. Other projects and
  event names are excluded.
- Counts represent stored event occurrences, not distinct transactions. Multiple
  logs in one transaction count separately.
- Time is the Unix timestamp in `mos.tx_timestamp`, not the insertion time or the
  event definition's creation time. NULL, zero and future timestamps are excluded.
- Each day uses `[midnight, next midnight)`. Today ends at the refresh timestamp
  (inclusive to the second).
- The window contains today and the preceding six calendar days. Days are sorted
  ascending. Chains are sorted numerically and their IDs are encoded as strings.
- Chain IDs come from `block`, `scan_block`, `event`, and matching occurrences
  inside the seven-day window. Every known chain appears on every day, with zero
  counts when no matching event occurred. Invalid or empty chain IDs are ignored.
- Soft-deleted event definitions still identify historical occurrences; physically
  deleted definitions cannot be matched.

## Response

The response contains seven entries in `data.days`. One day's shape is:

```json
{
  "date": "2026-09-16",
  "updated_at": 1789545600,
  "chains": [
    {"chain_id": "1", "message_out": 12, "message_in": 8},
    {"chain_id": "5042", "message_out": 0, "message_in": 0}
  ]
}
```

The full envelope is `{"code":200,"message":"success","data":...}`. The data
object also includes `timezone`, `start_date`, `end_date` (inclusive), and
`updated_at` (Unix seconds of the most recent successful refresh).

## Cache and Refresh

The API process warms an in-memory cache for the seven-day window at startup,
then refreshes today's counts at each hour in the configured timezone. Yesterday
is also refreshed through the first full hour after midnight to include its
final hour and allow delayed confirmations to arrive. Missing days
are filled, and days outside the window are discarded. Calendar boundaries use
the configured statistics timezone, including daylight saving transitions.

Requests only read the cache and never trigger database aggregation. Refreshes
have a five-minute timeout. Results are replaced atomically after a complete
successful refresh. If a refresh fails, existing results remain available with
their original `updated_at`; check that timestamp for freshness. If the current
seven-day window is not yet available, the endpoint returns HTTP 503 with
`code: 503` instead of reporting misleading zero counts. A subsequent hourly
refresh retries failed work.

The cache is local to each API process and rebuilt after restart. Historical
backfills or confirmations arriving more than one hour after a day ends are
reflected after a restart; finalized days are not queried every hour.

## Configuration and Index

The optional API config property `event_statistics_timezone` defaults to `UTC`.
Use `"event_statistics_timezone": "Asia/Shanghai"` for Beijing calendar days.
An invalid IANA timezone prevents API startup with a configuration error.

For an existing database, apply `init/mysql/event_statistics_index.sql` once
before enabling the endpoint. Fresh databases initialized with
`init/mysql/filter.sql` already have the covering index. The API does not execute
schema migrations automatically.
