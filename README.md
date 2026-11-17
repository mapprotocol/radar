# radar
MAP Protocol Radar is a service to filter all specified events on EVM and non-EVM chains.

## Container image

Successful pushes to `main` publish a multi-platform image to GHCR:

```text
ghcr.io/mapprotocol/radar:latest
ghcr.io/mapprotocol/radar:sha-<commit>
```

Tags matching `v*` publish the exact Git tag, for example
`ghcr.io/mapprotocol/radar:v1.2.3`. Every tag supports `linux/amd64` and
`linux/arm64`; Docker automatically selects the correct variant for the host.
Prefer a version or SHA tag in production so upgrades and rollbacks are
repeatable.

Public packages can be pulled directly:

```bash
docker pull ghcr.io/mapprotocol/radar:v1.2.3
```

For a private package, create a classic GitHub token with `read:packages` and
log in once on the deployment host:

```bash
export GHCR_TOKEN='<read-packages-token>'
echo "$GHCR_TOKEN" | docker login ghcr.io -u '<github-user>' --password-stdin
unset GHCR_TOKEN
```

The first workflow publication may create a private package. An organization
package administrator can make it public in the package settings when anonymous
pulls are required.

## Run with Docker

The image defaults to `filter cli --config /etc/radar/config.json`. Keep config
and keystore files on the host and mount them read-only:

```bash
docker pull ghcr.io/mapprotocol/radar:v1.2.3
docker run -d \
  --name radar \
  --restart unless-stopped \
  --env TZ=Asia/Shanghai \
  --publish 127.0.0.1:9101:9101 \
  --volume /srv/radar/config.json:/etc/radar/config.json:ro \
  --volume /srv/radar/keystore:/etc/radar/keystore:ro \
  ghcr.io/mapprotocol/radar:v1.2.3
```

The mounted files must be readable by the container's non-root user. Set
`keystore_path` in the JSON config to `/etc/radar/keystore` when that mount is
needed. Observability is available on the host loopback interface at port
`9101`.

To run the API command instead, override the default arguments and mount its
configuration. Set the mounted API config's `listen` value to `":8080"`; the
container port and host mapping below must match that value. If you use another
API port, change both values together:

```bash
docker run -d \
  --name radar-api \
  --restart unless-stopped \
  --publish 127.0.0.1:8080:8080 \
  --volume /srv/radar/api.json:/etc/radar/api.json:ro \
  ghcr.io/mapprotocol/radar:v1.2.3 \
  api --config /etc/radar/api.json
```

## Upgrade or roll back

Pull the selected immutable tag, remove the current container, and recreate it
with the same options:

```bash
docker pull ghcr.io/mapprotocol/radar:v1.2.4
docker stop radar
docker rm radar
docker run -d \
  --name radar \
  --restart unless-stopped \
  --env TZ=Asia/Shanghai \
  --publish 127.0.0.1:9101:9101 \
  --volume /srv/radar/config.json:/etc/radar/config.json:ro \
  --volume /srv/radar/keystore:/etc/radar/keystore:ro \
  ghcr.io/mapprotocol/radar:v1.2.4
```

Rollback uses the same sequence with the previous version tag. `latest` is
available for convenience, but immutable version or SHA tags are safer for
production deployment.

## Solana chain

`type: "solana"` ports the compass-ts Solana filter: it pages
`getSignaturesForAddress` (finalized) per program in `opts.mcs`, decodes the
Anchor `emit_cpi` events `CrossOutEvent` (chain pool) and `CrossInEvent` /
`RefundEvent` (receiver), and writes `mos` rows with topic `crossOut` /
`crossIn`, `block_number` = slot and the same `log_data` JSON as compass-ts.

```json
{
  "chains": [
    {
      "name": "solana",
      "type": "solana",
      "id": "1360108768460801",
      "endpoint": "https://<solana-rpc-endpoint>",
      "opts": {
        "mcs": [
          "mosv35NjuYkM8iMdihM96cG2ZziU2RJYZfSaGuV8uF6",
          "rcvr2wL88Bg3eBGH4Hr7VjugbDLm5YafBraggmmuRGJ"
        ],
        "startBlock": {
          "mosv35NjuYkM8iMdihM96cG2ZziU2RJYZfSaGuV8uF6": "<signature to start after>",
          "rcvr2wL88Bg3eBGH4Hr7VjugbDLm5YafBraggmmuRGJ": "<signature to start after>"
        },
        "butterEntrance": "butter+",
        "supportedBridgeMints": [
          "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
          "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU",
          "So11111111111111111111111111111111111111112"
        ],
        "nativeTokenOutAddress": "0x0000000000000000000000000000000000000000"
      }
    }
  ],
  "other": {
    "butter": "https://<butter-router-endpoint>",
    "butter_api_key": "<butter-api-key>"
  }
}
```

- `startBlock` may also be a single signature string used for every program.
  The cursor of each program is kept in `./blockStore/<program>.cursor` and
  wins over `startBlock` after the first run. An empty cursor scans the whole
  program history.
- `supportedBridgeMints` and `nativeTokenOutAddress` default to the values
  above. The butter API key can also come from the `BUTTER_API_KEY`
  environment variable.
- Rows are bound to events registered in the `event` table with
  `chain_id` = the Solana chain id, `address` = the program id that emits the
  event and the literal topic (`crossOut` / `crossIn`); one row is inserted per
  registered event. The event API hashes `format` into the topic, so register
  these rows with SQL:

  ```sql
  INSERT INTO event (project_id, chain_id, address, format, topic, block_number)
  VALUES (<project>, '1360108768460801', 'mosv35NjuYkM8iMdihM96cG2ZziU2RJYZfSaGuV8uF6', 'crossOut', 'crossOut', 'latest'),
         (<project>, '1360108768460801', 'rcvr2wL88Bg3eBGH4Hr7VjugbDLm5YafBraggmmuRGJ', 'crossIn', 'crossIn', 'latest');
  ```
- The scan height (`GET /v1/block/scan`) is the lowest slot up to which every
  signature of every program has been handled, and the latest height is the
  finalized slot. A transaction that keeps failing alarms and holds both the
  cursor and the scan height until it succeeds.
