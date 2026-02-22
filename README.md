# kb

A terminal Kanban board for personal project management. Lives in a tmux popup, designed for solo workflows with a data model ready for future sync with Jira, Linear, and GitHub Issues.

## Install

```bash
brew tap jeryldev/tap
brew install kb
```

Or build from source:

```bash
go install github.com/jeryldev/kb@latest
```

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

```bash
kb                              # Launch TUI (auto-detect board)
kb boards                       # List all boards
kb board create <name>          # Create board with default columns
kb board delete <name>          # Delete board

kb cards                        # List cards on current board
kb card add "Title"             # Add card to first column
kb card add -c "Todo" "Title"   # Add card to specific column
kb card move <id> "Done"        # Move card to column
kb card archive <id>            # Archive a card
kb card show <id>               # Show card details

kb columns                      # List columns for current board
kb column add "QA"              # Add column to current board
kb column reorder id1,id2,...   # Reorder columns by ID
```

Card IDs can be abbreviated to the first 4+ unique characters.

## Tmux Integration

With [dev-session-manager](https://github.com/jeryldev/dev-session-manager), press `prefix + k` to open kb in a tmux popup. The board auto-detects from your tmux session name.

## Data

Data is stored at `~/.local/share/kb/kb.db` (SQLite). Override with `$XDG_DATA_HOME`.

Default columns on board creation: Backlog, Todo, In Progress, Review, Done.

## License

MIT
