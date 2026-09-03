# GHCR Container Delivery Design

## Goal

Add a reproducible container build and a GitHub-hosted delivery workflow so a
deployment host can obtain Radar with `docker pull` instead of compiling the Go
binary locally.

The published image must support both `linux/amd64` and `linux/arm64`. The
current production host is `x86_64`, so Docker will select the `linux/amd64`
variant from the multi-platform image manifest.

## Scope

This change delivers images to GitHub Container Registry (GHCR). It does not
connect to a deployment host, restart a production process, or copy runtime
configuration. Deployment remains an explicit host-side pull and container
replacement operation.

The workflow uses GitHub-hosted standard Linux runners. Public repositories can
use standard GitHub-hosted runners without billed minutes, while private
repositories consume the owner's included Actions allowance. Public GHCR
packages can be pulled anonymously; private packages require a token with
`read:packages` permission.

## Container Image

Add a root-level multi-stage `Dockerfile`.

The build stage uses the Go version declared by the repository toolchain,
downloads modules before copying the remaining source for effective layer
caching, and cross-compiles with `CGO_ENABLED=0`, `TARGETOS`, and `TARGETARCH`.
The binary is built from `./cmd` with `-trimpath` and the existing version,
commit, and build-date linker variables.

The runtime stage uses a minimal non-root image that contains trusted CA
certificates and timezone data. The image contains only the `filter` binary and
runs as an unprivileged user. It exposes the CLI observability port `9101` as
documentation, without publishing it automatically.

The image entrypoint is the `filter` binary. Its default command is:

```text
cli --config /etc/radar/config.json
```

Operators can override the command to run the `api` subcommand. The image does
not contain application configuration, keystores, database credentials, or
other deployment secrets. Those files are mounted read-only at runtime and
must be readable by the image's non-root user.

Add a root-level `.dockerignore` to exclude Git metadata, local worktrees,
editor files, build output, coverage output, and local configuration or
keystore material from the Docker build context.

## Image Identity And Tags

The canonical image name is:

```text
ghcr.io/mapprotocol/radar
```

One manifest contains both supported platforms:

- `linux/amd64`
- `linux/arm64`

A push to the default branch publishes:

- `latest`
- `sha-<short-commit>`

A Git tag matching `v*` publishes:

- the exact Git tag, for example `v1.2.3`
- `sha-<short-commit>`

Production deployments should prefer an immutable version or SHA tag. The
`latest` tag is a convenience pointer to the most recent successful default
branch build.

OCI source, revision, created-time, and version labels link the package to the
GitHub repository and make the image provenance inspectable. The same revision,
version, and created time are injected into the Go binary through build
arguments.

## GitHub Actions Workflow

Add `.github/workflows/docker.yml` with these triggers:

- pull requests targeting `main`;
- pushes to `main`; and
- pushed Git tags matching `v*`.

The workflow has two dependent jobs:

1. `test` checks out the repository, configures the declared Go toolchain,
   restores the Go module/build cache, and runs `go test -count=1 ./...`.
2. `container` runs only after `test` succeeds. It configures QEMU and Docker
   Buildx, derives OCI labels and tags, builds both target platforms, and uses
   the GitHub Actions BuildKit cache.

For pull requests, the container job validates the Docker build but does not
authenticate to GHCR or push an image. For `main` and `v*` pushes, it logs in to
GHCR with the repository-scoped `GITHUB_TOKEN` and publishes the manifest.

Workflow permissions are least privilege:

- `contents: read` for checkout;
- `packages: write` only for publishing through GHCR; and
- no `pull_request_target` trigger, so untrusted pull-request code never runs
  with a write-capable base-repository context.

No Docker Hub credentials or custom repository secrets are required.

## GHCR Visibility

The first published GHCR package may be private. If anonymous host-side pulls
are required, an organization package administrator must change
`mapprotocol/radar` package visibility to public once in GitHub.

If the package remains private, the host performs a one-time login using a
classic personal access token with only `read:packages`:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u <github-user> --password-stdin
```

The token is supplied by the deployment environment and is never stored in this
repository.

## Deployment Flow

For the current `x86_64` host, Docker automatically resolves the AMD64 image:

```bash
docker pull ghcr.io/mapprotocol/radar:v1.2.3
docker run --rm ghcr.io/mapprotocol/radar:v1.2.3 --version
```

A CLI deployment mounts configuration and any required keystore read-only,
publishes observability locally, and applies an explicit restart policy. The
README will provide a concrete `docker run` example without embedding real
credentials or production paths.

Upgrading means pulling the desired immutable tag, stopping and removing the
old container, and starting a replacement with the same mounts and arguments.
Rollback means starting the previous immutable tag. Pulling `latest` is
supported for convenience but is not the recommended production rollback
strategy.

## Failure Handling

- A failing Go test prevents the image build and publish job.
- A failure for either target platform fails the combined publish; no new
  multi-platform tag is produced successfully.
- Pull-request builds cannot publish packages.
- A GHCR permission failure is visible as a failed workflow and does not alter
  the currently deployed container.
- Runtime configuration remains outside the image, so a missing or unreadable
  mount causes the application to exit rather than silently use baked-in
  secrets.

## Documentation

Extend `README.md` with:

- the canonical image and published tag policy;
- architecture behavior;
- public and private GHCR pull instructions;
- a `docker run` example for the `cli` subcommand;
- an example of overriding the command for `api`; and
- an explicit pull-and-replace upgrade procedure.

The examples use placeholders for configuration, keystore, credentials, ports,
and tags.

## Verification

Before completion:

- run `go test -count=1 ./...`;
- build the Dockerfile for `linux/amd64`;
- build the Dockerfile for `linux/arm64` through Buildx;
- run the AMD64 image's `--version` command where the local Docker daemon
  supports it;
- inspect the image to confirm the default entrypoint, command, exposed port,
  non-root user, and OCI labels;
- validate that ignored local files do not enter the build context; and
- validate the workflow syntax and event conditions locally as far as the
  available tooling permits.

The first pushed workflow run remains the authoritative end-to-end verification
of GitHub token permissions and GHCR publication because those services are not
available to local tests.

## Success Criteria

- Pull requests test the Go code and validate a multi-platform container build
  without publishing.
- Successful pushes to `main` publish `latest` and SHA tags to
  `ghcr.io/mapprotocol/radar`.
- Successful `v*` tags publish the exact version tag and SHA tag.
- Each published tag resolves on both `linux/amd64` and `linux/arm64`.
- The runtime image executes as a non-root user and contains no repository
  configuration or credentials.
- The binary reports the source version, commit, and build date supplied by the
  workflow.
- An operator can deploy or roll back using documented Docker pull-and-run
  commands without installing Go on the host.
