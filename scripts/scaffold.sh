#!/usr/bin/env bash
# scaffold.sh — clone internal/user/ into a new feature folder.
#
# Usage: scripts/scaffold.sh <Name>
# Example: scripts/scaffold.sh Order  →  creates internal/order/ with Order types.
#
# After scaffolding:
#   1. Trim event_handler.go / event_handler_test.go if the feature is sync-only.
#   2. Register the package in `.mockery.yaml` under `packages:`.
#   3. Add `<feature>.ProviderSet` reference in cmd/server/wire.go (and worker if async).
#   4. Wire routes in cmd/server/providers.go: ProvideRouter.
#   5. Run `make wire mocks test`.

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <Name>   (PascalCase singular, e.g. Order, Product, Invoice)" >&2
  exit 2
fi

NAME_PASCAL="$1"
NAME_LOWER="$(echo "$NAME_PASCAL" | tr '[:upper:]' '[:lower:]')"

if [[ ! "$NAME_PASCAL" =~ ^[A-Z][A-Za-z0-9]+$ ]]; then
  echo "error: <Name> must be PascalCase (got: $NAME_PASCAL)" >&2
  exit 2
fi

SRC="internal/user"
DST="internal/${NAME_LOWER}"

if [[ ! -d "$SRC" ]]; then
  echo "error: source template $SRC not found — run from repo root" >&2
  exit 1
fi
if [[ -e "$DST" ]]; then
  echo "error: destination $DST already exists" >&2
  exit 1
fi
if ! command -v perl >/dev/null; then
  echo "error: perl is required for portable rename" >&2
  exit 1
fi

echo "scaffolding $DST from $SRC..."
cp -R "$SRC" "$DST"

# Drop generated mocks — `make mocks` will regenerate from the new interfaces.
rm -rf "$DST/mocks"

# Rename identifiers everywhere inside the new folder. Unconditional replace
# (no word boundary) because Go word boundaries don't fire between adjacent
# letters: \b between `New` and `User` is empty, so \bUser wouldn't catch
# `NewUserRepository`. The user template has no 3rd-party identifiers
# containing the substring `user`/`User`, so naive replace is safe.
# Side effect: compound names like `UserName` become `${NAME_PASCAL}Name`,
# and comments mentioning "user" get renamed too — usually desirable.
find "$DST" -type f -name '*.go' -print0 | while IFS= read -r -d '' f; do
  perl -i -pe "
    s/User/${NAME_PASCAL}/g;
    s/user/${NAME_LOWER}/g;
  " "$f"
done

echo "done. next steps:"
echo "  1. edit $DST/model.go — adjust fields"
echo "  2. edit $DST/dto.go — adjust request/response shapes"
echo "  3. trim event_handler.go / event_handler_test.go if feature is sync-only,"
echo "     and remove NewEventHandler from $DST/wire.go's ProviderSet"
echo "  4. add to .mockery.yaml (interfaces ${NAME_PASCAL}Repository, ${NAME_PASCAL}Service)"
echo "  5. register ${NAME_LOWER}.ProviderSet in cmd/server/wire.go"
echo "  6. wire routes in cmd/server/providers.go (ProvideRouter)"
echo "  7. make wire mocks test"
