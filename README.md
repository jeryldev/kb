# kb

A terminal knowledge management tool for personal projects. Kanban boards, notes with wikilinks, workspace organization, and graph visualization — all in a tmux popup.

## Install

```bash
brew tap jeryldev/tap
brew install kb
```

Or build from source (requires Go 1.24+):

```bash
go install github.com/jeryldev/kb@latest
```

No external dependencies are required at runtime. The SQLite database is embedded via a pure-Go driver ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)) — no CGo or system libraries needed.

## Quick Start

```bash
# Create a workspace and board
kb workspace create my-project
kb board create sprint-1

# Add cards
kb card add "Fix login bug" -p urgent
kb card add "Add authentication" -c Todo -p medium

# Create a note with wikilinks
kb note create "Architecture decisions"
kb note edit architecture-decisions    # Opens $EDITOR

# Launch TUI
kb
```

## Features

### Kanban Boards

Full kanban board management with columns, cards, priorities, labels, and WIP limits.

### Notes and Wikilinks

Markdown notes with `[[wikilink]]` support. Link notes to each other, to cards (`[[card:Fix login bug]]`), or to boards (`[[board:sprint-1]]`). Backlinks are tracked automatically.

```bash
kb note create "Meeting notes"
kb note edit meeting-notes             # Opens $EDITOR
kb note backlinks meeting-notes        # Show what links to this note
kb note list --tag design              # Filter by tag
kb note search "authentication"        # Full-text search
```

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
kb publish setup my-blog --dir ~/blog/_posts
kb publish meeting-notes               # Export as Jekyll post
kb publish meeting-notes --draft       # Export as draft
kb publish meeting-notes --dry-run     # Preview without writing
kb publish list                        # Show publish history
```

Note: Republishing a note creates a new dated file without removing the previous version.

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
| `e` | Edit note in external editor |
| `d` | Delete note (with confirmation) |
| `Esc` | Back to workspace |

### Note Viewer

| Key | Action |
|-----|--------|
| `j` / `k` | Scroll content |
| `e` | Edit note in external editor |
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

# Notes
kb notes                                     # List notes
kb note create <title> [--tag "design,api"]  # Create note
kb note show <slug-or-id>                    # Show note content
kb note edit <slug-or-id>                    # Edit in $EDITOR
kb note delete <slug-or-id>                  # Delete note
kb note backlinks <slug-or-id>              # Show backlinks
kb notes --tag design                        # Filter by tag
kb notes --search "auth"                     # Search notes

# Graph
kb graph                                     # Text summary of connections
kb graph --open                              # Open HTML visualization in browser
kb graph --workspace <name>                  # Scope to workspace
kb graph --json                              # JSON node/edge data

# Publish
kb publish <slug> [--target name]            # Publish note as Jekyll post
kb publish <slug> --draft                    # Publish as draft
kb publish <slug> --dry-run                  # Preview without writing
kb publish setup <name> --dir <path>         # Create publish target
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

## Migrating from v0.1.x

If you are upgrading from v0.1.x (kanban-only):

- A "Default" workspace is automatically created and all existing boards are assigned to it
- No data is lost — boards and cards work exactly as before
- New features (notes, workspaces, graph, publish) are opt-in

## Tmux Integration

With [dev-session-manager](https://github.com/jeryldev/dev-session-manager), press `prefix + k` to open kb in a tmux popup. The board auto-detects from your tmux session name.

## Data

Data is stored at `~/.local/share/kb/kb.db` (SQLite). Override with `$XDG_DATA_HOME`.

Default columns on board creation: Backlog, Todo, In Progress, Review, Done.

## Known Limitations

- Graph visualization requires internet (D3.js loaded from CDN)
- Publish only supports Jekyll engine currently
- Republishing a note creates a new file without cleaning up the previous version
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
