#!/bin/sh
# Point the web UI at a GitHub Pages project path (https://<user>.github.io/<repo>/) instead of
# the /sod/ path the local server and wowsims.github.io use. CI-only: run on a fresh checkout
# before building the static site, never commit the result.
#   tools/forever/pages_rebase.sh wowforever-sim
set -e
NAME="$1"
[ -n "$NAME" ] || { echo "usage: $0 <repo name>"; exit 1; }
grep -rlZ --include='*.html' --include='*.ts' --include='*.tsx' '/sod/' ui | xargs -0 sed -i "s#/sod/#/$NAME/#g"
sed -i "s#export const REPO_NAME = 'sod';#export const REPO_NAME = '$NAME';#" ui/core/constants/other.ts
sed -i "s#base: '/sod/'#base: '/$NAME/'#" vite.config.mts
grep -q "REPO_NAME = '$NAME'" ui/core/constants/other.ts
grep -q "base: '/$NAME/'" vite.config.mts
echo "UI now served under /$NAME/"
