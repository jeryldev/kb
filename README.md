# kb

A terminal knowledge management tool for personal projects. Kanban boards, notes with wikilinks, workspace organization, and graph visualization — all in a tmux popup.

Notes are plain Markdown files in a folder you own (the vault), named by their titles, with YAML frontmatter, so they also open in any editor, in git, or in Obsidian on your phone. kb keeps a search and link index beside them that it can always rebuild from the files.

## Install

```bash
brew tap jeryldev/tap
brew install kb
```

Or build from source (requires Go 1.24+):

```bash
go install github.com/jeryldev/kb@latest
```

No external dependencies are required at runtime. The SQLite index is embedded via a pure-Go driver ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)) — no CGo or system libraries needed.

## Quick Start

```bash
# Create a workspace and board
kb workspace create my-project
kb board create sprint-1

# Add cards
kb card add "Fix login bug" -p urgent
kb card add "Add authentication" -c Todo -p medium

# Write notes
kb daily                               # Today's daily note in $EDITOR
kb note create "Architecture decisions"
kb open "architecture decisions"       # Open any note by name in $EDITOR

# Launch TUI
kb
```

## Features

### Kanban Boards

Full kanban board management with columns, cards, priorities, labels, and WIP limits.

### Notes and Wikilinks

Each note is a Markdown file in the vault (`$KB_VAULT`, default `~/notes`), named by its title, like `Meeting notes.md`. Edit the files with any editor, sync tool or app: kb rescans the vault every time it starts and picks up new, changed, moved and deleted files. A file with no frontmatter is a note too, so an existing Obsidian vault works as is, and kb never rewrites a file just by reading it.

Links work the way Obsidian reads them. `[[Meeting notes]]`, `[[meeting notes]]`, `[[folder/Meeting notes]]`, `[[Meeting notes#Decisions]]`, `[[Meeting notes|the meeting]]` and an alias from the note's `aliases:` all resolve to the same note, and a link to a note that does not exist yet starts working when you create it. Link to cards with `[[card:Fix login bug]]` and boards with `[[board:sprint-1]]`. Backlinks are tracked automatically.

```bash
kb daily                               # Open (or start) today's daily note
kb daily --date 2026-10-01             # Another day's
kb open "meeting notes"                # Open a note by title, alias, file name or slug
kb note rename meeting-notes "Q4 kickoff"   # Rename the file and rewrite links to it
kb note backlinks meeting-notes        # Show what links to this note
kb note search "authentication flow"   # Every word, as a prefix, best match first
kb notes --tag design                  # Filter by tag
kb tags                                # Tags, most used first
kb index                               # What the last scan found; --rebuild re-reads all
```

Frontmatter kb understands: `title` (when it differs from the file name), `tags`, `aliases`, `pinned`, `workspace`, `created` and `archived`. Other keys are kept untouched when kb edits a note.

### Workspaces

Organize boards and notes into workspaces using PARA kinds (projects, areas, resources, archives).

```bash
kb workspace create backend --kind project
kb workspace board move sprint-1 --workspace backend
kb workspace note move architecture-decisions --workspace backend
kb workspace show backend              # Lists boards and notes
```

### Graph Visualization

Visualize note connections as a force-directed graph in your browser.

```bash
kb graph                               # Text summary
kb graph --open                        # Open D3.js visualization in browser
kb graph --workspace backend           # Scope to workspace
kb graph --json                        # JSON node/edge data
```

Note: The HTML visualization loads D3.js from CDN and requires an internet connection.

### Publish to Jekyll

Export notes as Jekyll-compatible blog posts with front matter and resolved wikilinks.

```bash
kb publish setup my-blog --path ~/blog      # posts go in ~/blog/_posts (--posts-dir)
kb publish meeting-notes               # Export as Jekyll post
kb publish meeting-notes --draft       # Export as draft
kb publish meeting-notes --dry-run     # Preview without writing
kb publish list                        # Show publish history
```

A post is dated by the day its note was written (`created:`, on your local calendar), and publishing a note again updates the same post. Links to other notes become links to their posts when those are published and not drafts; otherwise they become plain text.

## TUI

Launch the interactive interface with `kb`. It auto-detects which board to open:

1. `$KB_BOARD` environment variable
2. Tmux session name (strips `dev-` prefix)
3. Current directory name
4. Falls back to workspace picker

### Workspace Picker

| Key | Action |
|-----|--------|
| `j` / `k` | Select workspace |
| `Enter` | Open workspace |
| `q` | Quit |

### Workspace Content

| Key | Action |
|-----|--------|
| `Tab` | Switch between boards and notes |
| `n` | Create new board or note |
| `d` | Delete (with confirmation) |
| `Enter` | Open selected board or note |
| `Esc` | Back to workspace picker |

### Board Keybindings

| Key | Action |
|-----|--------|
| `h` / `l` | Focus previous/next column |
| `j` / `k` | Select card up/down |
| `H` / `L` | Move card across columns |
| `J` / `K` | Reorder card within column |
| `n` | New card in current column |
| `Enter` | View card details |
| `e` | Edit card |
| `d` | Archive card (with confirmation) |
| `D` | Delete card (with confirmation) |
| `/` | Filter by label or priority |
| `1`-`4` | Filter by priority (1=urgent, 2=high, 3=medium, 4=low) |
| `b` | Switch board |
| `?` | Toggle help |
| `q` | Quit |

### Card Viewer

| Key | Action |
|-----|--------|
| `e` | Edit card |
| `d` | Archive card (with confirmation) |
| `D` | Delete card (with confirmation) |
| `Esc` / `q` | Back to board |

### Card Editor

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Navigate between fields |
| `h` / `l` | Cycle priority (when on priority field) |
| `Enter` | Save (from any field except Description) |
| `Esc` | Cancel |

### Note Browser

| Key | Action |
|-----|--------|
| `j` / `k` | Select note |
| `/` | Filter notes |
| `Enter` | View note |
| `e` | Edit the note's file in `$VISUAL` / `$EDITOR` |
| `d` | Delete note (with confirmation) |
| `Esc` | Back to workspace |

### Note Viewer

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll content |
| `e` | Edit the note's file in `$VISUAL` / `$EDITOR`; kb reloads it when you quit the editor |
| `Esc` / `q` | Back to note list |

## CLI Commands

All commands support `--json` for machine-readable output.

```bash
kb                                           # Launch TUI

# Workspaces
kb workspaces                                # List workspaces
kb workspace create <name> [--kind project]  # Create workspace (kinds: project, area, resource, archive)
kb workspace show <name>                     # Show workspace with boards and notes
kb workspace edit <name> [--kind area]       # Update workspace
kb workspace archive <name>                  # Archive workspace
kb workspace delete <name>                   # Delete (must be empty)
kb workspace board move <board> -w <ws>      # Move board to workspace
kb workspace note move <note> -w <ws>        # Move note to workspace

# Boards
kb boards                                    # List all boards
kb board create <name> [-d "description"]    # Create board with default columns
kb board delete <name> [-f]                  # Delete board

# Cards
kb cards                                     # List cards on current board
kb card add "Title" [-c column] [-p priority] [-d "desc"] [-l "a,b"] [-e EXT-1]
kb card show <id>                            # Show card details
kb card edit <id> [-t title] [-d desc] [-l labels] [-p priority] [-e ext-id]
kb card move <id> <column>                   # Move card to column
kb card archive <id>                         # Archive a card
kb card delete <id>                          # Soft-delete a card

# Columns
kb columns                                   # List columns for current board
kb column add <name>                         # Add column to current board
kb column delete <name> [-f]                 # Delete column and its cards
kb column wip-limit <name> <limit>           # Set WIP limit (0 to clear)
kb column reorder id1,id2,...                # Reorder columns by ID

# Notes (a <note> can be its title, alias, file name, slug or id)
kb notes                                     # List notes
kb note create <title> [--tags "design,api"] # Create note (file: "<title>.md")
kb note show <note>                          # Show note content
kb note edit <note> --title/--body/--tags    # Change fields
kb note rename <note> <new title>            # Rename file, rewrite links
kb note delete <note>                        # Delete note and its file
kb note backlinks <note>                     # Show backlinks
kb note search <words...>                    # Full-text search
kb notes --tag design                        # Filter by tag
kb open <note>                               # Edit the file in $EDITOR
kb daily [--date YYYY-MM-DD]                 # Daily note in $KB_DAILY_DIR (default daily/)
kb tags                                      # Tag counts
kb index [--rebuild]                         # Rescan the vault

# Graph
kb graph                                     # Text summary of connections
kb graph --open                              # Open HTML visualization in browser
kb graph --workspace <name>                  # Scope to workspace
kb graph --json                              # JSON node/edge data

# Publish
kb publish <slug> [--target name]            # Publish note as Jekyll post
kb publish <slug> --draft                    # Publish as draft
kb publish <slug> --dry-run                  # Preview without writing
kb publish setup <name> --path <site> [--posts-dir _posts]  # Create publish target
kb publish list                              # Show targets and publish log
kb publish delete <target-name>              # Remove publish target
```

Card IDs and note slugs can be abbreviated to the first 4+ unique characters. Column and workspace names are case-insensitive.

### Flags Reference

| Flag | Short | Commands | Description |
|------|-------|----------|-------------|
| `--json` | | all | Output in JSON format |
| `--description` | `-d` | board create, card add, card edit | Description text |
| `--column` | `-c` | card add | Target column (default: first) |
| `--priority` | `-p` | card add, card edit | low, medium, high, urgent |
| `--title` | `-t` | card edit | New title |
| `--labels` | `-l` | card add, card edit | Comma-separated labels |
| `--external-id` | `-e` | card add, card edit | External system ID (Jira, GitHub, etc.) |
| `--force` | `-f` | board delete, column delete | Skip confirmation prompt |
| `--kind` | `-k` | workspace create, workspace edit | PARA kind: project, area, resource, archive |
| `--workspace` | `-w` | workspace board move, workspace note move | Target workspace |
| `--tag` | | note create, notes list | Comma-separated tags |
| `--search` | | notes list | Search note titles and bodies |
| `--target` | | publish | Publish target name |
| `--draft` | | publish | Publish as draft |
| `--dry-run` | | publish | Preview without writing files |
| `--open` | | graph | Open visualization in browser |

## AI Tool Integration

All CLI commands support `--json` for structured output, making kb scriptable by AI tools (Claude Code, Gemini, etc.) and shell pipelines.

```bash
# List boards
kb boards --json

# Create a card with all fields
kb card add "Fix auth bug" -p urgent -l "bug,security" -e "GH-42" --json

# Pipeline example: list all urgent cards
kb cards --json | jq '[.[] | select(.priority == "urgent")]'

# Note operations
kb note create "Sprint retro" --tag "retro,sprint-3" --json
kb note backlinks sprint-retro --json
```

In `--json` mode, destructive commands (delete) skip interactive confirmation prompts, making them safe for non-interactive use.

## Migrating from v0.2.x

Notes used to live only inside the SQLite database. The first time kb 0.3 runs, it writes each of them out as a Markdown file in the vault (keeping its id, tags and dates) and from then on reads the files. Set `KB_VAULT` before that first run if you want them somewhere other than `~/notes`. Boards and cards are unchanged.

## Migrating from v0.1.x

If you are upgrading from v0.1.x (kanban-only):

- A "Default" workspace is automatically created and all existing boards are assigned to it
- No data is lost — boards and cards work exactly as before
- New features (notes, workspaces, graph, publish) are opt-in

## Tmux Integration

With [dev-session-manager](https://github.com/jeryldev/dev-session-manager), press `prefix + k` to open kb in a tmux popup. The board auto-detects from your tmux session name.

## Data

Notes are the Markdown files in the vault: `$KB_VAULT`, default `~/notes`. Daily notes go in `$KB_DAILY_DIR` inside it, default `daily`.

Boards, cards and workspaces, plus the note index (search, links, tags), are in `~/.local/share/kb/kb.db` (SQLite; override with `$XDG_DATA_HOME`). Keep it out of a synced folder; deleting it loses no notes, though the index has to be rebuilt and boards and cards live only there.

Default columns on board creation: Backlog, Todo, In Progress, Review, Done.

## Known Limitations

- Graph visualization requires internet (D3.js loaded from CDN)
- Publish only supports Jekyll engine currently
- Archived workspaces remain visible in list commands (no `--active` filter yet)

## Dependencies

kb is a single static binary with no runtime dependencies.

**Build dependencies** (managed via `go.mod`):

| Dependency | Purpose |
|-----------|---------|
| [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | Pure-Go SQLite driver (no CGo required) |
| [spf13/cobra](https://github.com/spf13/cobra) | CLI framework |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | Terminal UI framework |
| [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss) | TUI styling |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | TUI components (text input, viewport) |
| [google/uuid](https://github.com/google/uuid) | UUID generation for entity IDs |

## License

MIT License - see [LICENSE](LICENSE) for details.

## Author

[Jeryl Donato Estopace](https://www.linkedin.com/in/jeryldev/) ([@jeryldev](https://github.com/jeryldev))
