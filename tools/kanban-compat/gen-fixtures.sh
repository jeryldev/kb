#!/bin/sh
# Regenerates internal/board/testdata/plugin from testdata/src by running
# every source board through the plugin's own parser and writer (harness.js,
# see build.sh). Each source gives two files:
#   plugin/<name>.md    the board as the plugin saves it
#   plugin/<name>.json  the lanes and cards the plugin reads from that save
#   plugin/<name>.src.json  what the plugin reads from the source itself
# A ".usetab" or ".<locale>" part in the name sets USE_TAB=1 or KB_LANG.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
data="$here/../../internal/board/testdata"
mkdir -p "$data/plugin"
for src in "$data"/src/*.md; do
  name=$(basename "$src" .md)
  usetab=0
  lang=en
  case "$name" in *.usetab*) usetab=1 ;; esac
  case "$name" in *.de) lang=de ;; *.ja) lang=ja ;; esac
  USE_TAB=$usetab KB_LANG=$lang node "$here/harness.js" roundtrip < "$src" > "$data/plugin/$name.md"
  USE_TAB=$usetab KB_LANG=$lang node "$here/harness.js" json < "$data/plugin/$name.md" > "$data/plugin/$name.json"
  USE_TAB=$usetab KB_LANG=$lang node "$here/harness.js" json < "$src" > "$data/plugin/$name.src.json"
done
echo "regenerated $(ls "$data/plugin"/*.md | wc -l | tr -d ' ') fixtures"
