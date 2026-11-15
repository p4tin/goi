#!/bin/sh
# Creates the sample Python project used by pyi-smoke.json:
#   tools/e2e/pyi-sample.sh <dir>
set -e
d="${1:?usage: pyi-sample.sh <dir>}"
mkdir -p "$d/pkg"
cat > "$d/pyproject.toml" <<'PY'
[project]
name = "pyi-sample"
version = "0.1.0"
PY
cat > "$d/pkg/__init__.py" <<'PY'
PY
cat > "$d/pkg/util.py" <<'PY'
def double(n: int) -> int:
    """Returns twice n."""
    return n * 2
PY
cat > "$d/main.py" <<'PY'
import os

from pkg.util import double


def main() -> None:
    print("hello from pyi", double(21))


if __name__ == "__main__":
    main()
PY
