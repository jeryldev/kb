# kb

Notes and Kanban boards for your terminal, kept as plain Markdown files: notes with wikilinks, boards in the format of Obsidian's Kanban plugin, workspaces, a link graph and publishing to Jekyll. It runs well in a tmux popup.

Everything kb keeps is a file in a folder you own, the vault. Notes are Markdown named by their titles, boards are Markdown that the [Obsidian Kanban plugin](https://github.com/mgmeyers/obsidian-kanban) opens as boards, and workspaces are listed in the vault's `.kb/workspaces.yml`. There is no database: kb reads the vault when it starts, so any editor, sync tool, git or Obsidian on your phone can change the files too.

## Install

```bash
brew tap jeryldev/tap
brew install kb
```

Or build from source (Go 1.25+):

```bash
go install github.com/jeryldev/kb@latest
```

kb is a single binary with no runtime dependencies. Upgrading from 0.3 also needs the `sqlite3` program once, for `kb import` (macOS has it).

## Quick start

```bash
export KB_VAULT=~/notes                # where everything lives (the default)

kb workspace create backend --kind project
kb board create sprint-1 -w backend    # Boards/sprint-1.md

kb card add "Fix login bug" -B sprint-1 -p urgent
kb card add "Add authentication" -B sprint-1 -c Todo -l "auth, backend"

kb note create "Architecture decisions" -w backend
kb open "architecture decisions"       # edit the file in $EDITOR
kb daily                               # today's daily note

kb                                     # the TUI
```

Card and column commands work on one board: `--board`/`-B`, or `$KB_BOARD`, or else the first board that exists named after the dev tmux session (`$TMUX_SESSION_NAME`), the folder you are in, or its git repository (also from a worktree). So inside `~/code/sprint-1` plain `kb cards` is enough.

## Built-in help

`kb help start` walks you through your first notes, a link between them and a board. Other guides:

| Command | Covers |
|---|---|
| `kb help links` | connecting notes with `[[wikilinks]]`: headings, aliases, display text, links from cards, backlinks, the graph |
| `kb help kanban` | boards, columns and cards, and how they are stored |
| `kb help tui` | the keys of the full-screen view |
| `kb help files` | where kb keeps things, the frontmatter it reads, its settings |
| `kb help scripting` | JSON output, confirmations and exit codes |

Every command has examples: `kb <command> --help`, for example `kb card add --help`.

## Notes and wikilinks

Each note is a Markdown file in the vault (`$KB_VAULT`, default `~/notes`), named by its title, like `Meeting notes.md`. A file with no frontmatter is a note too, so an existing Obsidian vault works as is, and kb never rewrites a file just by reading it.

Links work the way Obsidian reads them. `[[Meeting notes]]`, `[[meeting notes]]`, `[[folder/Meeting notes]]`, `[[Meeting notes#Decisions]]`, `[[Meeting notes|the meeting]]` and an alias from the note's `aliases:` all resolve to the same note, and a link to a note that does not exist yet starts working when you create it. Cards can link to notes too, and `[[sprint-1]]` links a board. Links inside code are not links.

```bash
kb daily                               # open (or start) today's daily note
kb daily --date 2026-10-01             # another day's
kb daily --json                        # the note's JSON, without opening an editor
kb open "meeting notes"                # open a note by title, alias, file name or slug
kb note rename "meeting notes" "Q4 kickoff"   # rename the file and rewrite links to it
kb note backlinks "q4 kickoff"         # notes and cards that link to it
kb note search authentication flow     # every word, as a prefix, accents ignored
kb notes --tag design                  # filter by tag (--search too)
kb tags                                # tags, most used first
kb note delete "old idea"              # moves the file to the vault's .trash/
kb index                               # what kb finds in the vault, and any problems
```

Frontmatter kb understands: `title` (when it differs from the file name), `tags`, `aliases`, `pinned`, `workspace`, `created`, `archived` and `published`. Other keys are kept as they are when kb edits a note.

## Kanban boards

A board is a Markdown file in the Obsidian Kanban plugin's format, made in the vault's `Boards/` folder (`$KB_BOARDS_DIR`). Any file with `kanban-plugin: board` in its frontmatter is a board, wherever it is, so boards you made in Obsidian work in kb and the other way round.

```markdown
---
kanban-plugin: board
workspace: backend
---

## Todo

- [ ] Add authentication #auth #backend ^3f9a1c2e

## In Progress (2)

- [ ] Fix login bug #priority/urgent [ext:: GH-42] ^889962bb
    The session cookie expires too early.
```

- Columns are headings; a number in parentheses is the column's WIP limit.
- Labels are `#tags`. Spaces, and characters a tag cannot hold (`#`, `&`, `+`, `.` and other punctuation), become dashes: "needs review" is `#needs-review`, "q&a" is `#q-a`. kb reads tags as the Kanban plugin does: `#c++` is the tag `c`, and a `#word` in a code span is code, not a tag.
- A card's id is its block id, the `^id` the plugin reads: the last `^word` ending a line of the card. A `^` that would be read that way in text kb writes is escaped (`mc\^2`). A card with no `^id` gets one kb derives from its column and title, written into the file once kb changes the board when another card shares that title.
- Priority is a `#priority/high` tag. Medium is the default and is not written.
- The text after `^` is the card's id. It is unique within its board, and kb finds a card by it, or by its first 4 or more characters, on any board.
- An external id (Jira, GitHub, Linear) is a `[ext:: …]` field.
- Archived cards go under the board's Archive heading, as the plugin does.

```bash
kb boards                              # every board in the vault
kb board create sprint-2 -d "Q4"       # with Backlog, Todo, In Progress, Review, Done
kb board move sprint-2 -w backend      # into a workspace (no -w: Default)
kb board delete sprint-2               # moves the file to .trash/ (asks first)

kb cards                               # cards on the current board
kb cards -p urgent -l auth -c Todo -s login   # filters
kb card add "Write tests" -c Todo -p high -d "unit and e2e" -l "qa" -e GH-7
kb card show 889962bb
kb card edit 8899 -t "Fix the login bug" -p high
kb card move 8899 Done                 # to the end of Done
kb card move 8899 Todo --before 3f9a   # above another card
kb card archive 8899
kb card delete 8899                    # removes it from the board (asks first)

kb columns
kb column add QA
kb column rename QA Testing
kb column reorder "Backlog,Todo,In Progress,Testing,Review,Done"   # every column, once
kb column reorder Backlog Todo "In Progress, QA" Done              # names with commas: one per argument
kb column wip-limit "In Progress" 2    # 0 clears it
kb column delete Testing               # its cards are archived (asks first)
```

Adding or moving a card into a column at its WIP limit needs `--force`.

## Workspaces

Workspaces group notes and boards, with PARA kinds (project, area, resource, archive). They are listed in the vault's `.kb/workspaces.yml`, so they sync with the vault; a note or board names its workspace in its frontmatter, and one that names none is in Default.

```bash
kb workspace                           # list (also: kb workspaces, kb ws)
kb workspace create backend --kind project -d "API work" -p ~/code/api
kb workspace show backend              # its boards and notes
kb workspace edit backend --name api   # the new name is written into its notes and boards
kb workspace archive api
kb workspace delete api                # only when nothing uses it
kb note move "meeting notes" -w backend
```

A workspace's `--path` is a folder on this machine, so it is kept in `~/.config/kb`, not in the vault.

## Graph

```bash
kb graph                               # a summary
kb graph --open                        # an interactive D3.js graph in the browser
kb graph -w backend                    # one workspace, plus what it links to elsewhere
kb graph --json                        # nodes (notes, boards, cards) and edges
```

The HTML graph has D3.js in the page, so it opens offline and fetches nothing; `--open` writes it to `~/.cache/kb/graph.html`. Links between two items, however many and whichever way, are one line in the graph.

## Publish to Jekyll

```bash
kb publish setup blog --path ~/blog    # posts go in ~/blog/_posts (--posts-dir)
kb publish setup blog --path ~/blog --permalink "/:year/:month/:day/:title/"
kb publish "meeting notes"             # write it as a post
kb publish "meeting notes" --draft     # published: false
kb publish "meeting notes" --dry-run   # show it, write nothing
kb publish list                        # the sites
kb publish list -t blog                # a site's posts
kb publish delete blog                 # forget the site (its posts stay)
kb publish -- list                     # a note named like a subcommand goes after --
```

A post is dated by the day its note was written (`created:`, on your local calendar), and publishing a note again updates the same post. Where each note was published is kept in its frontmatter (`published:`), written before the post, so a publish that fails part way is mended by publishing again. Sites are kept in `~/.config/kb/publish.yml`, since their paths are paths on this machine. With several sites and no `--target`, a note goes to the one site set up for its workspace (`setup -w`).

- Links to other notes become links to their posts when those are out, and plain text otherwise. A link to a heading goes to the heading on the post, and with no display text reads "Note > Heading". Links start with the site's `baseurl` from its `_config.yml`.
- The permalink pattern (`--permalink`, default `/blog/:year/:month/:day/:title/`) can use `:year`, `:month`, `:day` and `:title`; kb refuses anything else (Jekyll's named styles such as `pretty`, or `:categories`).
- An embedded note (`![[Note]]`) reads like a link to it, an embedded file by its name.
- A note holding `{{ }}` or `{% %}` is wrapped in `{% raw %}`, so Jekyll shows it as written instead of running it.
- Tags YAML would read as something else (`true`, `null`, `2024`, `yes`) are quoted.
- A note in iCloud that is not downloaded is not published: kb has only its name.

## TUI

Run `kb` to open the TUI. It opens the current board (see Quick start) when there is one, and otherwise the workspace picker. Errors and messages show in a line above the key hints until the next key.

### Workspace picker

| Key | Action |
|-----|--------|
| `j` / `k` | Select workspace |
| `Enter` | Open workspace |
| `q` | Quit |

### Workspace

| Key | Action |
|-----|--------|
| `j` / `k` | Select a board or note |
| `Enter` | Open it |
| `n` | New board |
| `N` | New note |
| `d` | Move the board or note to the trash (asks first) |
| `b` / `Esc` | Back to the picker |
| `q` | Quit |

### Board

| Key | Action |
|-----|--------|
| `h` / `l` | Focus previous/next column |
| `j` / `k` | Select card down/up |
| `H` / `L` | Move card across columns |
| `J` / `K` | Reorder card within column |
| `n` | New card in this column |
| `Enter` | View card |
| `e` | Edit card |
| `d` | Archive card (asks first) |
| `D` | Delete card (asks first) |
| `/` | Filter by text or label |
| `1`-`4` | Show only urgent, high, medium or low |
| `Esc` | Clear the filters |
| `r` | Read the board again from its file |
| `b` | Back to the workspace |
| `?` | Help |
| `q` | Quit |

The board follows its file: when an editor, the Kanban plugin or another kb changed it, the next key shows the change (or, once a filter is typed or a move or question is done, the key after). A board whose file is moved or deleted says so and goes back to the workspace.

In the card editor, a field you leave alone is saved exactly as the file had it. If a card changed on disk while you edited it, saving stops and says so: the fields you typed in keep your text, the others now show the change made elsewhere. Save again to keep that, or `Esc` to drop your edits; `Esc` on a form you typed in asks first. A card gone from the file meanwhile can be saved as a new card. A full column asks before taking one more card.

### Card viewer

| Key | Action |
|-----|--------|
| `e` | Edit card |
| `d` / `D` | Archive / delete card (asks first) |
| `Esc` / `b` / `q` | Back to the board |

### Card editor

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Next/previous field |
| `h` / `l` | Change priority (on the priority field) |
| `Enter` | Save (in the description, `Ctrl+S`) |
| `Esc` | Cancel, back where you came from |

### Note viewer

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll |
| `e` | Edit the note's file in `$VISUAL` / `$EDITOR`; kb reads it again when the editor exits |
| `d` | Move the note to the trash (asks first) |
| `b` / `Esc` | Back to the workspace |
| `q` | Quit |

## Scripts and AI tools

Every listing and change can print JSON with `--json`, for scripts, AI tools and `jq`. A list with nothing in it prints `[]`.

```bash
kb boards --json
kb card add "Fix auth bug" -B api -p urgent -l "bug,security" -e GH-42 --json
kb cards -B api --json | jq '[.[] | select(.priority == "urgent")]'
kb note backlinks "sprint retro" --json
```

The JSON fields:

- **note**: `id`, `title`, `slug`, `path`, `body`, `tags` (a list), `pinned`, `workspace`, `workspace_id`, `created_at`, `updated_at`
- **board**: `id` (its vault path), `name`, `description`, `workspace`
- **card**: `id`, `board`, `column`, `title`, `description`, `priority`, `labels` (a list), `external_id`, `archived`
- **column**: `name`, `position`, `wip_limit`, `cards`
- **workspace**: `id`, `name`, `kind`, `description`, `path`, `position`; `kb workspace show --json` adds `boards` (names) and `notes` (slugs)
- `kb note delete --json` adds `trashed`, where the file went; `kb daily --json` prints the note instead of opening it. `kb import` prints text only.
- Times are RFC 3339 in this machine's time zone.
- **publish**: `note`, `target`, `file_path` (in the site), `full_path` (on this machine), `draft`; `--dry-run --json` adds `content`
- **graph**: `nodes` (`id`, `label`, `type`, `slug`, `workspace_id`, `connections`, `outside`) and `edges` (`source`, `target`, `context`); a card's node id is `<board path>#<card id>`

Commands that delete ask on stderr and read the answer from stdin. With no answer (a script) they stop with an error, so a script passes `--force` (`-f`). Errors exit with status 1.

## Upgrading from 0.3

kb 0.3 kept boards, cards and workspaces in a SQLite database (`~/.local/share/kb/kb.db`). kb 0.4 keeps them as files, and moves them over once:

```bash
kb import --dry-run      # what it will do, writing nothing
kb import
```

Each board becomes a file in `Boards/`, keeping the start of each card's id; archived cards go to the board's Archive, and deleted ones are dropped. Workspaces go to `.kb/workspaces.yml`, publish sites to `~/.config/kb/publish.yml` and publish history into the notes' frontmatter. Labels with spaces become tags with dashes, and the report lists each one. Everything is checked before anything is written, and running it again skips what is done. Afterwards `kb.db` is renamed `kb.db.imported-0.4` and kept. Until the import, kb asks you to run it. A `kb.db` that is empty or not a database (with no `kb.db-wal` beside it) holds nothing to import and does not stop kb; an SQLite file that is not kb 0.3's says so, and how to move it aside.

The JSON output changed: card `labels` and note `tags` are lists, boards and cards are named by name rather than by uuid, and cards have no timestamps.

## Tmux

With [dev-session-manager](https://github.com/jeryldev/dev-session-manager), `prefix + k` opens kb in a tmux popup, in the window's folder, so it opens that project's board: the first board that exists named after the dev session (`$TMUX_SESSION_NAME` without its `dev-`), the folder, or its git repository, worktrees included. If kb cannot start, the popup stays open until you press Enter, so you can read why.

## Files

| What | Where |
|------|-------|
| Notes | `$KB_VAULT` (default `~/notes`), any folder |
| Daily notes | `$KB_DAILY_DIR` in the vault (default `daily/`) |
| Boards | any `kanban-plugin: board` file; new ones in `$KB_BOARDS_DIR` (default `Boards/`) |
| Workspaces | `.kb/workspaces.yml` in the vault |
| Deleted notes and boards | `.trash/` in the vault, as Obsidian does |
| Publish sites, workspace folders | `~/.config/kb` (`$XDG_CONFIG_HOME`) |
| Locks that keep two kb processes from clobbering a file | `~/.cache/kb/locks` (`$XDG_CACHE_HOME`) |

kb writes each file atomically, under a lock, and an edit to a note or card that changed on disk since kb read it is refused rather than overwrite the other change. The lock is the same however the vault is named (a symlink, or another letter case on macOS). A settings file kb cannot read (`workspaces.yml`, `publish.yml`) is reported as a warning and never written over until you fix it; changing a workspace edits only its entry in `workspaces.yml`, keeping comments and keys kb does not know. A note's frontmatter that kb did not change is written back as it was, comments, quoting and line endings included. A note in iCloud that is not downloaded shows by name, with a warning, and is never overwritten.

## Limitations

- A board open in Obsidian while kb changes it: the Kanban plugin saves the board as it holds it a moment after any change made in Obsidian, so a kb change made in between can be lost. Close the board in Obsidian (or wait for it to reload the file) before changing it from kb.
- Publishing supports Jekyll only.
- Archived workspaces still show in lists.

## Dependencies

| Dependency | Purpose |
|-----------|---------|
| [spf13/cobra](https://github.com/spf13/cobra) | Commands and flags |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea), [lipgloss](https://github.com/charmbracelet/lipgloss), [bubbles](https://github.com/charmbracelet/bubbles) | The TUI |
| [yuin/goldmark](https://github.com/yuin/goldmark) | Reading boards' Markdown as the Kanban plugin does |
| [go-yaml/yaml](https://github.com/go-yaml/yaml) | Frontmatter and settings |
| [golang.org/x/text](https://pkg.go.dev/golang.org/x/text) | Search that ignores accents |
| [google/uuid](https://github.com/google/uuid) | Note ids |

## License

MIT License - see [LICENSE](LICENSE) for details.

## Author

[Jeryl Donato Estopace](https://www.linkedin.com/in/jeryldev/) ([@jeryldev](https://github.com/jeryldev))
