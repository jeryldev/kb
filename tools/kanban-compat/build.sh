#!/bin/sh
# Builds harness.js from a checkout of github.com/mgmeyers/obsidian-kanban
# whose dependencies are installed (yarn install --frozen-lockfile).
# KB_LANG=de (or any plugin locale) runs the harness as Obsidian in that
# language, which changes the **Complete** and Archive markers.
# The banner adds the Array helpers Obsidian puts on every array (first,
# last, contains, remove) and the browser globals the bundle touches when
# it loads.
#   tools/kanban-compat/build.sh /path/to/obsidian-kanban
set -eu
plugin=$1
here=$(cd "$(dirname "$0")" && pwd)
"$plugin/node_modules/.bin/esbuild" "$here/harness.ts" --bundle --platform=node --format=cjs \
  --log-level=warning \
  --tsconfig="$plugin/tsconfig.json" \
  --alias:obsidian="$here/obsidian-stub.ts" \
  --alias:obsidian-dataview="$here/dataview-stub.ts" \
  --alias:moment="$plugin/node_modules/moment" \
  --alias:yaml="$plugin/node_modules/yaml" \
  --loader:.css=empty \
  --banner:js="Array.prototype.first = function () { return this[0]; }; Array.prototype.last = function () { return this[this.length - 1]; }; Array.prototype.contains = function (x) { return this.includes(x); }; Array.prototype.remove = function (x) { var i = this.indexOf(x); if (i >= 0) this.splice(i, 1); }; globalThis.window = globalThis.window || { localStorage: { getItem: function () { return process.env.KB_LANG || 'en'; } }, navigator: { userAgent: 'node' }, addEventListener: function () {} }; globalThis.navigator = globalThis.navigator || window.navigator; globalThis.document = globalThis.document || { createElement: function () { return { style: {}, classList: { add: function () {} }, setAttribute: function () {}, appendChild: function () {} }; }, head: { appendChild: function () {} }, addEventListener: function () {}, documentElement: { style: {} } };" \
  --outfile="$here/harness.js"
echo "built $here/harness.js"
