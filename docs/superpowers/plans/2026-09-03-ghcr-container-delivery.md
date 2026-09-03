# GHCR Container Delivery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Radar as a non-root multi-platform container and publish tested `linux/amd64` and `linux/arm64` images to GHCR from GitHub Actions.

**Architecture:** A root multi-stage Dockerfile cross-compiles the existing `./cmd` entrypoint into a static binary and copies it into a minimal non-root runtime. One GitHub Actions workflow runs Go tests first, validates container builds for pull requests, and publishes a shared multi-platform manifest for `main` and `v*` pushes. Runtime configuration remains host-mounted and the README documents authenticated or anonymous pull-and-replace deployment.

**Tech Stack:** Go 1.24.1, Docker BuildKit/Buildx, OCI images, GitHub Actions, GitHub Container Registry (GHCR)

**Spec:** `docs/superpowers/specs/2026-09-03-ghcr-container-delivery-design.md`

## Global Constraints

- Publish the canonical image as `ghcr.io/mapprotocol/radar`.
- Every published tag must contain `linux/amd64` and `linux/arm64` variants.
- The current production host is `x86_64` and must receive the AMD64 variant automatically.
- Never copy runtime configuration, keystores, database credentials, or registry credentials into the final image.
- Run the final container as a non-root user.
- Preserve the existing CLI and API command interface; the default container command is `cli --config /etc/radar/config.json`.
- Pull requests may test and build but must never authenticate to GHCR or publish images.
- Default-branch pushes publish `latest` plus a SHA tag; `v*` pushes publish the exact Git tag plus a SHA tag.
- Do not add SSH deployment or remotely restart production hosts.

---

### Task 1: Reproducible Multi-Platform Container

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

**Interfaces:**
- Consumes: the Go module in `go.mod`, the executable package at `./cmd`, and linker variables `internal/version.Version`, `internal/version.Commit`, and `internal/version.BuildDate`.
- Produces: a BuildKit-compatible image with build arguments `VERSION`, `COMMIT`, and `BUILD_DATE`; entrypoint `/usr/local/bin/filter`; default arguments `cli --config /etc/radar/config.json`; user `nonroot`; and exposed port `9101/tcp`.

- [ ] **Step 1: Verify the packaging files do not exist yet**

Run both commands; each must exit non-zero because the repository has no
container packaging yet:

```bash
test -f Dockerfile
test -f .dockerignore
```

- [ ] **Step 2: Add the multi-stage Dockerfile**

Create `Dockerfile`:

```dockerfile
# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.24.1-alpine AS build

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath \
      -ldflags="-s -w -X github.com/mapprotocol/filter/internal/version.Version=$VERSION -X github.com/mapprotocol/filter/internal/version.Commit=$COMMIT -X github.com/mapprotocol/filter/internal/version.BuildDate=$BUILD_DATE" \
      -o /out/filter ./cmd

FROM gcr.io/distroless/static-debian12:nonroot

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="MAP Protocol Radar" \
      org.opencontainers.image.description="MAP Protocol cross-chain event filter" \
      org.opencontainers.image.source="https://github.com/mapprotocol/radar" \
      org.opencontainers.image.version=$VERSION \
      org.opencontainers.image.revision=$COMMIT \
      org.opencontainers.image.created=$BUILD_DATE

COPY --from=build /out/filter /usr/local/bin/filter

USER nonroot:nonroot
EXPOSE 9101

ENTRYPOINT ["/usr/local/bin/filter"]
CMD ["cli", "--config", "/etc/radar/config.json"]
```

The build stage matches `toolchain go1.24.1`. The distroless static runtime
supplies CA certificates and timezone data without a shell or package manager.

- [ ] **Step 3: Limit the Docker build context**

Create `.dockerignore`:

```dockerignore
.git
.github
.idea
.worktrees
.DS_Store
**/.DS_Store
build
coverage
*.cover
*.out
*.log
tmp.txt
config*.json
keystore*
```

- [ ] **Step 4: Build and execute one local AMD64 image**

```bash
docker buildx build --platform linux/amd64 --load \
  --build-arg VERSION=test \
  --build-arg COMMIT=local \
  --build-arg BUILD_DATE=2026-09-03T00:00:00Z \
  --tag radar:container-test .
docker run --rm --platform linux/amd64 radar:container-test --version
```

Expected: output contains `test`, `commit=local`, and
`build_date=2026-09-03T00:00:00Z`.

- [ ] **Step 5: Inspect the runtime contract**

```bash
docker image inspect radar:container-test \
  --format 'user={{.Config.User}} entrypoint={{json .Config.Entrypoint}} cmd={{json .Config.Cmd}} ports={{json .Config.ExposedPorts}} source={{index .Config.Labels "org.opencontainers.image.source"}}'
```

Expected:

```text
user=nonroot:nonroot entrypoint=["/usr/local/bin/filter"] cmd=["cli","--config","/etc/radar/config.json"] ports={"9101/tcp":{}} source=https://github.com/mapprotocol/radar
```

- [ ] **Step 6: Build the combined AMD64/ARM64 OCI archive**

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --output type=oci,dest=/tmp/radar-multiarch.tar \
  --build-arg VERSION=test \
  --build-arg COMMIT=local \
  --build-arg BUILD_DATE=2026-09-03T00:00:00Z .
```

Expected: both target platforms build and `/tmp/radar-multiarch.tar` exists.

- [ ] **Step 7: Run checks and commit the container**

```bash
go test -count=1 ./...
git diff --check
git add Dockerfile .dockerignore
git commit -m "build: add multi-platform container image"
```

Expected: all checks pass and the commit succeeds.

---

### Task 2: Tested GHCR Publishing Workflow

**Files:**
- Create: `.github/workflows/docker.yml`

**Interfaces:**
- Consumes: the Task 1 Dockerfile build arguments and image contract, the repository `GITHUB_TOKEN`, and GitHub event fields `github.event_name`, `github.ref`, `github.repository`, and `github.sha`.
- Produces: PR build validation without publication; `ghcr.io/mapprotocol/radar:latest`; `ghcr.io/mapprotocol/radar:sha-<short-commit>`; and exact pushed `v*` tags, each as an AMD64/ARM64 manifest.

- [ ] **Step 1: Verify the workflow does not exist yet**

```bash
test -f .github/workflows/docker.yml
```

Expected: exit code is non-zero.

- [ ] **Step 2: Add the test-before-publish workflow**

Create `.github/workflows/docker.yml`:

```yaml
name: Container

on:
  pull_request:
    branches:
      - main
  push:
    branches:
      - main
    tags:
      - "v*"

concurrency:
  group: container-${{ github.ref }}
  cancel-in-progress: true

permissions:
  contents: read

env:
  IMAGE_NAME: ghcr.io/${{ github.repository }}

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - name: Check out repository
        uses: actions/checkout@v6

      - name: Set up Go
        uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
          cache: true

      - name: Run tests
        run: go test -count=1 ./...

  container:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - name: Check out repository
        uses: actions/checkout@v6

      - name: Set up QEMU
        uses: docker/setup-qemu-action@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v4

      - name: Generate image metadata
        id: meta
        uses: docker/metadata-action@v6
        with:
          images: ${{ env.IMAGE_NAME }}
          tags: |
            type=raw,value=latest,enable={{is_default_branch}}
            type=sha,prefix=sha-
            type=ref,event=tag

      - name: Record build date
        id: build
        shell: bash
        run: echo "date=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" >> "$GITHUB_OUTPUT"

      - name: Log in to GHCR
        if: github.event_name != 'pull_request'
        uses: docker/login-action@v4
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and optionally publish image
        uses: docker/build-push-action@v7
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VERSION=${{ steps.meta.outputs.version }}
            COMMIT=${{ github.sha }}
            BUILD_DATE=${{ steps.build.outputs.date }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

The login and push conditions must remain identical. Do not use
`pull_request_target`, repository PAT secrets, or Docker Hub credentials.

- [ ] **Step 3: Validate GitHub Actions syntax**

```bash
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/docker.yml
```

Expected: no output and exit code zero.

- [ ] **Step 4: Verify publication guards, tags, and platforms**

```bash
rg -n "Log in to GHCR|github.event_name != 'pull_request'|type=raw,value=latest|type=sha|type=ref,event=tag|linux/amd64,linux/arm64" .github/workflows/docker.yml
rg -n "pull_request_target" .github/workflows/docker.yml
```

Expected: the first command finds both non-PR guards, all tag rules, and both
platforms. The second command exits one because `pull_request_target` is absent.

- [ ] **Step 5: Rebuild with the Actions build-argument contract**

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  --output type=oci,dest=/tmp/radar-workflow-test.tar \
  --build-arg VERSION=sha-local \
  --build-arg COMMIT=local \
  --build-arg BUILD_DATE=2026-09-03T00:00:00Z .
```

Expected: both platforms build with all workflow-supplied arguments.

- [ ] **Step 6: Run checks and commit the workflow**

```bash
go test -count=1 ./...
git diff --check
git add .github/workflows/docker.yml
git commit -m "ci: publish multi-platform image to GHCR"
```

Expected: all checks pass and the workflow commit succeeds.

---

### Task 3: Deployment Documentation And Final Verification

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: image tags and runtime defaults from Tasks 1 and 2.
- Produces: exact operator commands for public/private pulls, AMD64 deployment, API command override, upgrades, and rollback.

- [ ] **Step 1: Verify the README does not document the image yet**

```bash
rg -n "ghcr.io/mapprotocol/radar|docker pull|docker run" README.md
```

Expected: no matches.

- [ ] **Step 2: Document image tags and GHCR authentication**

Append to `README.md`:

````markdown
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
configuration:

```bash
docker run -d \
  --name radar-api \
  --restart unless-stopped \
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
````

- [ ] **Step 3: Verify documented commands match the image contract**

```bash
rg -n "ghcr.io/mapprotocol/radar|linux/amd64|linux/arm64|read:packages|/etc/radar/config.json|/etc/radar/keystore|api --config|docker pull|docker stop|docker rm" README.md
docker run --rm --platform linux/amd64 radar:container-test --version
```

Expected: all contract terms are found and the image reports the injected test
version.

- [ ] **Step 4: Run complete repository and delivery verification**

```bash
go test -count=1 ./...
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7 .github/workflows/docker.yml
docker buildx build --platform linux/amd64,linux/arm64 \
  --output type=oci,dest=/tmp/radar-final-multiarch.tar \
  --build-arg VERSION=final-test \
  --build-arg COMMIT=local \
  --build-arg BUILD_DATE=2026-09-03T00:00:00Z .
git diff --check
git status --short
```

Expected: Go tests pass, Actionlint emits no errors, both platform builds
succeed, the diff check is empty, and only `README.md` is uncommitted.

- [ ] **Step 5: Commit the operator documentation**

```bash
git add README.md
git commit -m "docs: document container deployment"
git status --short --branch
```

Expected: the commit succeeds and the feature worktree is clean.

- [ ] **Step 6: Record the external verification boundary**

After pushing the implementation branch, inspect the first GitHub Actions run
and confirm GHCR contains `linux/amd64` and `linux/arm64` manifests for the
expected tags. Local tests cannot prove GitHub-hosted token permissions or
organization package visibility.
