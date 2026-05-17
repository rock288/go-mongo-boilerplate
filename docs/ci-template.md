# CI Template (deferred)

This repo ships **local quality gates** (`make ci`) only. Drop the GitHub Actions workflow below into `.github/workflows/ci.yml` when you're ready to enforce them on PRs.

## Pre-flight

Pin all action versions to SHAs (or at least to a major). Pin `golangci-lint-action` to the version matched in `Makefile` (`GOLANGCI_LINT_VERSION`).

## `.github/workflows/ci.yml`

```yaml
name: ci
on:
  pull_request:
    branches: [main]
  push:
    branches: [main]

permissions: {}

jobs:
  secret-scan:
    permissions: { contents: read }
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: gitleaks/gitleaks-action@v2
        env: { GITLEAKS_LICENSE: ${{ secrets.GITLEAKS_LICENSE }} }

  lint:
    permissions: { contents: read }
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - uses: golangci/golangci-lint-action@v6
        with: { version: v1.64.0 }

  test:
    permissions: { contents: read }
    runs-on: ubuntu-24.04
    services:
      mongo:
        image: mongo:7
        ports: ['27017:27017']
        options: >-
          --health-cmd "echo 'db.runCommand({ping:1}).ok' | mongosh --quiet | grep 1"
          --health-interval 5s
          --health-retries 10
      redpanda:
        image: redpandadata/redpanda:v24.2.4
        ports: ['9092:9092']
        options: --health-cmd "rpk cluster health --exit-when-healthy"
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go install -tags 'mongodb' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
      - run: make migrate-up
        env:
          MONGO_URI: mongodb://localhost:27017
          MONGO_DATABASE: go_mongo_boilerplate
      - run: make test-race

  vuln:
    permissions: { contents: read }
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version-file: go.mod }
      - run: go install golang.org/x/vuln/cmd/govulncheck@latest
      - run: govulncheck ./...

  docker:
    permissions: { contents: read, packages: write, id-token: write }
    needs: [lint, test, vuln, secret-scan]
    runs-on: ubuntu-24.04
    strategy:
      matrix:
        target: [server, worker, migrate]
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - uses: docker/build-push-action@v6
        with:
          context: .
          push: false
          build-args: |
            TARGET=${{ matrix.target }}
            VERSION=${{ github.sha }}
            COMMIT=${{ github.sha }}
          tags: ${{ matrix.target }}:${{ github.sha }}
          load: true
      - uses: aquasecurity/trivy-action@0.24.0
        with:
          image-ref: ${{ matrix.target }}:${{ github.sha }}
          severity: CRITICAL,HIGH
          exit-code: 1
      - uses: anchore/sbom-action@v0
        with:
          image: ${{ matrix.target }}:${{ github.sha }}
          format: spdx-json
          output-file: sbom-${{ matrix.target }}.spdx.json
      # Optionally sign with cosign once registry is configured:
      # - uses: sigstore/cosign-installer@v3
      # - run: cosign sign --yes <registry>/${{ matrix.target }}:${{ github.sha }}
```

## Steps to enable

1. Copy the workflow into `.github/workflows/ci.yml`.
2. Add a branch protection rule on `main` requiring: `secret-scan`, `lint`, `test`, `vuln`, all `docker (server|worker|migrate)` checks.
3. Optional: add a `.github/dependabot.yml` for gomod + actions + docker bumps.
