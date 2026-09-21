#!/usr/bin/env bash
# 同步上游更新并合并进自有分支。
# 详见 docs/custom/README.md
set -euo pipefail

REMOTE="${REMOTE:-upstream}"
BASE_BRANCH="${BASE_BRANCH:-main}"
WORK_BRANCH="${WORK_BRANCH:-custom}"

cd "$(dirname "$0")/.."

if ! git rev-parse --git-dir >/dev/null 2>&1; then
    echo "错误：当前目录不是 git 仓库" >&2
    exit 1
fi

if ! git show-ref --verify --quiet "refs/heads/$WORK_BRANCH"; then
    echo "错误：找不到分支 $WORK_BRANCH" >&2
    exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
    echo "错误：工作区有未提交的改动，请先提交或 stash 后再同步。" >&2
    git status --short
    exit 1
fi

start_branch="$(git rev-parse --abbrev-ref HEAD)"
echo "起始分支：$start_branch"

echo "-> 拉取 $REMOTE ..."
git fetch "$REMOTE" --tags --prune

echo "-> 更新 ${BASE_BRANCH}（仅允许快进）..."
git checkout "$BASE_BRANCH"
git merge --ff-only "$REMOTE/$BASE_BRANCH"

echo "-> 把上游更新合并进 $WORK_BRANCH ..."
git checkout "$WORK_BRANCH"
git merge "$BASE_BRANCH" --no-edit

if [ "$start_branch" != "$WORK_BRANCH" ]; then
    echo "-> 切回 $start_branch"
    git checkout "$start_branch"
fi

echo
echo "完成。上游更新已并入 ${WORK_BRANCH}。"
