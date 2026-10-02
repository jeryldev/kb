// Runs the Obsidian Kanban plugin's own Markdown parser and writer on a
// board file, so kb's board format can be checked against the real thing.
//
//   node harness.js roundtrip < board.md   # parse and save, as the plugin would
//   node harness.js json < board.md        # the lanes and cards the plugin sees
//
// USE_TAB=1 simulates Obsidian's "indent using tabs" setting.
import { astToUnhydratedBoard, boardToMd } from 'src/parsers/formats/list';
import { parseMarkdown } from 'src/parsers/parseMarkdown';

// KB_PLUGINS=dataview,obsidian-tasks-plugin simulates those plugins being
// enabled; the Kanban plugin only reads inline fields when they are.
const enabled = new Set((process.env.KB_PLUGINS || '').split(',').filter(Boolean));
(globalThis as any).app = {
  vault: { getConfig: (key: string) => (key === 'useTab' ? process.env.USE_TAB === '1' : undefined) },
  plugins: { enabledPlugins: enabled, plugins: {} },
};

const defaults: Record<string, unknown> = {
  'date-trigger': '@',
  'time-trigger': '@@',
  'date-format': 'YYYY-MM-DD',
  'time-format': 'HH:mm',
  'date-display-format': 'YYYY-MM-DD',
  'inline-metadata-position': 'body',
};

function stateManager() {
  let settings: Record<string, unknown> = {};
  return {
    file: { path: 'board.md' },
    app: { metadataCache: { getFirstLinkpathDest: () => null } },
    state: undefined,
    hasError: () => false,
    compileSettings(s: Record<string, unknown>) {
      settings = s;
    },
    getSetting(key: string) {
      return settings[key] ?? defaults[key];
    },
  };
}

const md = require('fs').readFileSync(0, 'utf8');
const sm: any = stateManager();
const { ast, settings, frontmatter } = parseMarkdown(sm, md);
const board = astToUnhydratedBoard(sm, settings as any, frontmatter, ast, md);

if (process.argv[2] === 'json') {
  const item = (i: any) => ({
    titleRaw: i.data.titleRaw,
    checkChar: i.data.checkChar,
    blockId: i.data.blockId ?? null,
    tags: i.data.metadata.tags ?? [],
  });
  process.stdout.write(
    JSON.stringify(
      {
        frontmatter,
        settings,
        lanes: board.children.map((l: any) => ({
          title: l.data.title,
          maxItems: l.data.maxItems,
          complete: l.data.shouldMarkItemsComplete,
          items: l.children.map(item),
        })),
        archive: board.data.archive.map(item),
      },
      null,
      2
    ) + '\n'
  );
} else {
  process.stdout.write(boardToMd(board));
}
