# kb

A terminal Kanban board for personal project management. Lives in a tmux popup, designed for solo workflows with a data model ready for future sync with Jira, Linear, and GitHub Issues.

## Install

```bash
brew tap jeryldev/tap
brew install kb
```

Or build from source (requires Go 1.25+):

```bash
go install github.com/jeryldev/kb@latest
```

No external dependencies are required at runtime. The SQLite database is embedded via a pure-Go driver ([modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)) — no CGo or system libraries needed.

## Quick Start

```bash
# Create a board
kb board create my-project

# Add cards
kb card add "Fix login bug" -p urgent
kb card add "Add authentication" -c Todo -p medium

# Launch TUI
kb
```

## Usage

### TUI

Launch the interactive board with `kb`. It auto-detects which board to open:

1. `$KB_BOARD` environment variable
2. Tmux session name (strips `dev-` prefix)
3. Current directory name
4. Falls back to board picker

### Keybindings

| Key | Action |
|-----|--------|
| `h` / `l` | Focus previous/next column |
| `j` / `k` | Select card up/down |
| `H` / `L` | Move card across columns |
| `J` / `K` | Reorder card within column |
| | *(then `h/l/j/k` to position, `Enter` to confirm)* |
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

### Board Picker

| Key | Action |
|-----|--------|
| `j` / `k` | Select board |
| `Enter` | Open board |
| `n` | Create new board |
| `d` | Delete board (with confirmation) |
| `q` | Quit |

### Card Editor

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Navigate between fields |
| `h` / `l` | Cycle priority (when on priority field) |
| `Enter` | Save (from any field except Description) |
| `Esc` | Cancel |

### CLI Commands

All commands support `--json` for machine-readable output (see [AI Tool Integration](#ai-tool-integration)).

```bash
kb                                           # Launch TUI (auto-detect board)

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
```

Card IDs can be abbreviated to the first 4+ unique characters. Column names are case-insensitive.

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

## AI Tool Integration

All CLI commands support `--json` for structured output, making kb scriptable by AI tools (Claude Code, Gemini, etc.) and shell pipelines.

```bash
# List boards
kb boards --json
# [{"id":"...","name":"my-project","description":"","created_at":"...","updated_at":"..."}]

# Create a card with all fields
kb card add "Fix auth bug" -p urgent -l "bug,security" -e "GH-42" --json
# {"id":"...","column":"Backlog","title":"Fix auth bug","priority":"urgent","labels":"bug,security",...}

# Edit specific fields (only changed fields are updated)
kb card edit abc1 --priority high --labels "bug" --json

# Move card and get updated state
kb card move abc1 "In Progress" --json

# Check column WIP limits
kb columns --json
# [{"id":"...","name":"In Progress","position":2,"wip_limit":3,"cards":2},...]

# Set WIP limit
kb column wip-limit "In Progress" 5 --json

# Pipeline example: list all urgent cards
kb cards --json | jq '[.[] | select(.priority == "urgent")]'
```

In `--json` mode, destructive commands (delete) skip interactive confirmation prompts, making them safe for non-interactive use.

## Tmux Integration

With [dev-session-manager](https://github.com/jeryldev/dev-session-manager), press `prefix + k` to open kb in a tmux popup. The board auto-detects from your tmux session name.

## Data

Data is stored at `~/.local/share/kb/kb.db` (SQLite). Override with `$XDG_DATA_HOME`.

Default columns on board creation: Backlog, Todo, In Progress, Review, Done.

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

MIT
