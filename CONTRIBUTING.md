# Contributing

Thanks for considering a contribution. This is a Go boilerplate intended as a public template — keep changes small, focused, and idiomatic.

## Workflow

1. Fork the repo and create a feature branch off `main`.
2. Make your change. Keep PRs scoped: one concern per PR.
3. Run `make ci` locally — must be green before opening a PR.
4. If you changed an interface, regenerate:
   ```bash
   make wire
   make mocks
   ```
5. Open a PR against `main`. Fill in the PR template checklist.

## Commit style

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` new feature
- `fix:` bug fix
- `chore:` tooling, deps, no production code change
- `docs:` docs only
- `refactor:` code restructure, no behavior change
- `test:` test-only changes

Example: `feat(user): add cursor pagination on List endpoint`.

## Tests

Every public function in `internal/` should have a unit test. Follow the go-kit style described in `CLAUDE.md` — table-driven, mockery for interfaces, one test file per production file.

Integration tests (testcontainers, real Mongo, real Kafka) are intentionally **not** in this boilerplate. Build them in your downstream repo when you have a real feature to test.

## Linting

`make lint` runs `golangci-lint` v2. Fix violations before pushing; CI will block merges otherwise.

## Questions

Open an issue using the appropriate template (bug / feature).
