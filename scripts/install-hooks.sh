#!/usr/bin/env bash
# Point git at the version-controlled hooks in .githooks/. Run once after clone.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
git config core.hooksPath .githooks
chmod +x .githooks/* 2>/dev/null || true
echo "✓ Git hooks installed (core.hooksPath -> .githooks)."
echo "  The pre-commit secret scan now runs on every commit."
