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
