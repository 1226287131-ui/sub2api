#!/usr/bin/env bash
set -euo pipefail

TARGET="${1:-upstream/main}"
CURRENT_BRANCH="$(git symbolic-ref --quiet --short HEAD || true)"
if [[ -z "$CURRENT_BRANCH" ]]; then
  echo "error: detached HEAD is not supported" >&2
  exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is not clean" >&2
  exit 1
fi
if ! git remote get-url upstream >/dev/null 2>&1; then
  git remote add upstream https://github.com/Wei-Shaw/sub2api.git
fi

git config rerere.enabled true
git config rerere.autoupdate true
git fetch upstream --tags --prune
TARGET_COMMIT="$(git rev-parse "${TARGET}^{commit}")"
OLD_BASE="$(git merge-base HEAD "$TARGET_COMMIT")"
if [[ "$OLD_BASE" == "$TARGET_COMMIT" ]]; then
  echo "already based on $TARGET ($TARGET_COMMIT)"
  exit 0
fi

STAMP="$(date +%Y%m%d-%H%M%S)"
SAFE_BRANCH="${CURRENT_BRANCH//\//-}"
BACKUP_BRANCH="archive/${SAFE_BRANCH}-before-${STAMP}"
git branch "$BACKUP_BRANCH" HEAD

echo "backup: $BACKUP_BRANCH"
echo "rebasing custom commits after $OLD_BASE onto $TARGET_COMMIT"
if ! git rebase --onto "$TARGET_COMMIT" "$OLD_BASE" "$CURRENT_BRANCH"; then
  cat >&2 <<MSG
rebase stopped on conflicts.
Resolve files, then run:
  git add <resolved-files>
  git rebase --continue
To roll back:
  git rebase --abort
  git reset --hard $BACKUP_BRANCH
MSG
  exit 1
fi

echo "updated $CURRENT_BRANCH onto $TARGET"
echo "run backend/frontend tests before deploying"
