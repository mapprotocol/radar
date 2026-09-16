# Listening Events

`GET /v1/event/listening` returns all active event definitions applicable to one
chain, across all projects. It uses the existing `/v1` API key and IP whitelist
authentication.

```bash
curl 'http://localhost:8080/v1/event/listening' \
  -H 'X-API-Key: <api-key>'

curl 'http://localhost:8080/v1/event/listening?chain_id=5042' \
  -H 'X-API-Key: <api-key>'
```

## Query

- Omit `chain_id` to query chain `1`.
- Supply exactly one positive decimal integer, up to `18446744073709551615`.
  Leading zeroes are normalized, so `0001` selects chain `1`.
- Empty, zero, negative, non-integer, overflow, `all`, comma-separated and
  repeated `chain_id` values return HTTP 400 with `code: 400`.
- There is no all-chain mode or pagination; the response includes every
  applicable event definition for the selected chain.

## Selection and Freshness

Each request reads `event` directly. It includes rows for the selected chain and
global definitions (`chain_id IS NULL` or `chain_id = ''`), matching the chain
selection used by the Ethereum and XRP listeners. Rows with non-NULL
`deleted_at` are excluded. Events are ordered by ID ascending, with no project
filter and no signature deduplication: the same signature can be monitored at
multiple contract addresses or by multiple projects.

This is a view of the current database configuration, not a live process
snapshot. The API and filter run separately and do not share the listener's
in-memory state. New definitions may take a listener refresh cycle to load;
the existing Ethereum listener checks every 30 seconds. Existing listeners
append definitions and do not remove already loaded rows after database
deletion or updates. This endpoint does not change that lifecycle or verify
that a chain's scanner is running. Non-EVM listeners may have additional
chain-specific rules.

## Response

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "chain_id": "5042",
    "total": 1,
    "list": [
      {
        "id": 170,
        "project_id": 8,
        "address": "0x0001805c0B57DBd48B5c5c26E237a135dDC678ae",
        "format": "ClientNotifySend(address,uint256,bytes)",
        "topic": "0x7063ee7ac21ca792eb7d62d3a65598a5c986c4b0f7bd701aa453eb8a1387c956",
        "block_number": "latest",
        "created": 1755775128
      }
    ]
  }
}
```

`data.chain_id` is always the selected chain, including when global definitions
are present. `created` is the definition's Unix creation timestamp, or `0` when
absent. `block_number` is the configured starting block, not scan progress.
An empty result returns HTTP 200, `total: 0` and `list: []`.
An unknown chain may still return global definitions; otherwise its list is
empty. Database failures return HTTP 500 with `code: 500` and a generic message.

No database migration is required.
