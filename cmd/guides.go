package cmd

import "github.com/spf13/cobra"

// Guides are help topics: kb help <topic>. They have no Run, so cobra
// lists them under "Additional help topics".

var startGuide = &cobra.Command{
	Use:   "start",
	Short: "Getting started: your first notes, a link between them, a board",
	Long: `Getting started with kb

1. Where your notes live

   Everything kb keeps is a file in your vault: the folder $KB_VAULT names
   (default ~/notes). Check which folder kb uses, and what is in it:

     kb index

   To use another folder, set it in ~/.zshenv (so every shell sees it):

     export KB_VAULT="$HOME/notes"

2. Write a note

     kb note create "Cash and Cash Equivalents" --tags far,assets \
       --body "Cash includes coins, currency and demand deposits."

   This makes "Cash and Cash Equivalents.md" in the vault. Open it in your
   editor ($VISUAL or $EDITOR) with:

     kb open "cash and cash equivalents"

3. Connect two notes with a link

   A link is the other note's title in double square brackets:

     kb note create "Bank Reconciliation" \
       --body "Adjusts the [[Cash and Cash Equivalents]] balance to the bank's."

   See what links to a note, and the whole web of links:

     kb note backlinks "cash and cash equivalents"
     kb graph
     kb graph --open        (an interactive picture in your browser)

   More on links, headings, aliases and links from cards: kb help links

4. Find things

     kb notes                    every note, newest first
     kb notes --tag far          notes with a tag
     kb note search bank         notes with all these words
     kb tags                     tags, most used first
     kb daily                    today's daily note

5. Track work on a board

     kb board create study
     kb card add "Review FAR 01" -B study -p high -l far
     kb cards -B study
     kb card move <id> "In Progress" -B study

   A card's <id> is shown by kb cards; its first 4 characters are enough.
   More on boards: kb help kanban

6. The full-screen view

   Run kb on its own in a terminal window to browse workspaces, boards and
   notes with the keyboard. Its keys: kb help tui

More guides: kb help links, kb help kanban, kb help tui, kb help files,
kb help scripting. Every command has examples: kb <command> --help.`,
}

var linksGuide = &cobra.Command{
	Use:   "links",
	Short: "Connecting notes and boards with [[wikilinks]], backlinks and the graph",
	Long: `Links between notes ("connections")

A link is a name in double square brackets, written anywhere in a note's
text, or in a card's title or description:

  [[Cash and Cash Equivalents]]          a note, by its title
  [[cash and cash equivalents]]          case does not matter
  [[Cash and Cash Equivalents|cash]]     shown as "cash"
  [[Cash and Cash Equivalents#Petty]]    a heading in that note
  [[FAR/Cash and Cash Equivalents]]      by file path, for same-named notes
  [[study]]                              a board, by its name

How kb finds what a link names, first match wins: the note's slug, its
title, its file name or path, one of its aliases, then the slugified name;
boards by name come after notes. Notes are files, so these are the same
rules Obsidian uses.

Aliases: other names a note answers to, in its frontmatter:

  ---
  aliases: [Cash, CCE]
  ---

Now [[CCE]] links to that note.

A link to a note that does not exist yet is fine: it starts working when
you create the note.

Making links

  kb note create "Petty Cash" --body "Part of [[Cash and Cash Equivalents]]."
  kb note edit "petty cash" --body "See [[Bank Reconciliation]] too."
  kb open "petty cash"                         (write links in your editor)
  kb card add "Read [[Petty Cash]]" -B study   (a card that links to a note)

Following links

  kb note backlinks "cash and cash equivalents"   notes and cards linking here
  kb note show "petty cash"                       the note, links and all
  kb graph                                        how many notes, links, orphans
  kb graph --open                                 the picture, in a browser
  kb graph -w Work                                one workspace, and its links out

Renaming keeps links working: kb note rename renames the file and rewrites
every [[link]] to the old name, in notes and cards.

  kb note rename "petty cash" "Petty Cash Fund"

Links inside code (between backticks, or in a fenced code block) are
text, not links.`,
}

var kanbanGuide = &cobra.Command{
	Use:   "kanban",
	Short: "Boards, columns and cards, and how they are stored",
	Long: `Boards, columns and cards

A board is a Markdown file in the format of Obsidian's Kanban plugin, kept
in the vault's Boards folder ($KB_BOARDS_DIR, default "Boards"). Any file
with "kanban-plugin: board" in its frontmatter is a board, wherever it is.

  kb board create study -d "Exam prep"   with Backlog, Todo, In Progress,
                                         Review and Done columns
  kb boards                              every board in the vault
  kb board move study -w Work            into a workspace
  kb board delete study                  to the vault's .trash (asks first)

Which board a command works on: --board/-B, else $KB_BOARD, else the
board named after the folder you are in or after its git repository.

Cards

  kb card add "Review FAR 01" -B study -c Todo -p high -l "far, review" \
    -d "Chapters 1 to 3" -e LINEAR-42
  kb cards -B study                      the cards, column by column
  kb cards -B study -p high -l far       filter by priority, label, column
                                         (-c) or words (-s)
  kb card show <id> -B study
  kb card edit <id> -B study -t "Review FAR 01 to 03" -p urgent
  kb card move <id> Done -B study        to the end of Done
  kb card move <id> Todo --before <other id> -B study   above a card in Todo
  kb card archive <id> -B study          to the board's Archive
  kb card delete <id> -B study           off the board (asks first)

A card's <id> is the "^id" at the end of its line in the file; kb accepts
its first 4 characters or more, and finds it on any board.

Priority is low, medium, high or urgent; medium is the default. Labels
become #tags, with spaces as dashes ("needs review" is #needs-review).

Columns

  kb columns -B study
  kb column add QA -B study
  kb column rename QA Testing -B study
  kb column reorder "Backlog,Todo,In Progress,Testing,Review,Done" -B study
  kb column wip-limit "In Progress" 2 -B study     0 clears the limit
  kb column delete Testing -B study                its cards are archived

A column at its WIP limit refuses more cards unless you add --force.

In the file, a column is a heading ("## In Progress (2)" has a limit of
2), a card is a "- [ ]" line, labels are #tags, priority is a
#priority/high tag, and an external id is an [ext:: ...] field.`,
}

var tuiGuide = &cobra.Command{
	Use:   "tui",
	Short: "Keys of the full-screen view (run kb alone in a terminal)",
	Long: `The full-screen view

Run kb with no command in a terminal window. It opens the current board
(see kb help kanban), or else the workspace picker. Messages and errors
show in a line above the keys until you press another key.

Workspace picker
  j / k      select              Enter   open
  q          quit

Workspace
  j / k      select a board or note
  Enter      open it             n       new board      N   new note
  d          move it to the trash (asks first)
  b / Esc    back to the picker  q       quit

Board
  h / l      previous / next column
  j / k      next / previous card
  H / L      move the card to another column
  J / K      move the card up or down its column
             (then h j k l to place it, Enter to confirm, Esc to cancel)
  n          new card            Enter   view card      e   edit card
  d          archive card        D       delete card    (both ask first)
  /          filter by words or label (Esc stops typing, keeps the filter)
  1 2 3 4    only urgent / high / medium / low cards (again to clear)
  Esc        clear the filters   b       back           ?   help
  q          quit

Card view
  e          edit                d / D   archive / delete
  Esc, b, q  back to the board

Card editor
  Tab, Shift+Tab   next / previous field
  h / l            change the priority (on the priority field)
  Enter            save (in the description, Ctrl+S)
  Esc              cancel

Note view
  j / k      scroll              e       edit in $VISUAL or $EDITOR
  d          move to the trash   b / Esc back           q   quit

The view needs a real terminal window. Inside a tool that runs commands
without one (an editor's task runner, an AI assistant), use the commands
instead: kb notes, kb boards, kb cards.`,
}

var filesGuide = &cobra.Command{
	Use:   "files",
	Short: "Where kb keeps things, the frontmatter it reads, and its settings",
	Long: `Files and settings

kb keeps no database: it reads the vault every time it runs, so any editor,
sync service, git or Obsidian can change the files too.

In the vault ($KB_VAULT, default ~/notes)
  any .md file         a note, named by its title ("Petty Cash.md")
  daily/               daily notes, YYYY-MM-DD.md ($KB_DAILY_DIR)
  Boards/              new boards ($KB_BOARDS_DIR)
  .kb/workspaces.yml   the workspaces
  .trash/              deleted notes and boards, as Obsidian does

A note's frontmatter, all optional:
  ---
  title: Petty Cash          when it differs from the file name
  tags: [far, assets]
  aliases: [PCF]             other names links can use (kb help links)
  workspace: Work            Default when absent
  pinned: true               listed first
  created: 2026-10-02        dates posts made with kb publish
  ---
kb also writes id (so a renamed file keeps its history), archived and
published (where kb publish put it). Other keys are left as they are.

On this machine only
  ~/.config/kb/publish.yml            sites set up with kb publish setup
  ~/.config/kb/workspace-paths.yml    workspace folders (kb workspace -p)
  ~/.cache/kb/locks                   locks that keep two kb runs apart

Environment
  KB_VAULT        the vault folder
  KB_BOARD        the board card and column commands use
  KB_BOARDS_DIR   where kb board create puts boards ("Boards")
  KB_DAILY_DIR    where kb daily puts daily notes ("daily")
  VISUAL, EDITOR  the editor kb open and kb daily start
  XDG_CONFIG_HOME, XDG_CACHE_HOME   instead of ~/.config and ~/.cache

Set them in ~/.zshenv rather than ~/.zshrc: .zshrc is read only by
interactive shells, so tmux popups and scripts would not see them.`,
}

var scriptingGuide = &cobra.Command{
	Use:   "scripting",
	Short: "JSON output, confirmations and exit codes, for scripts and AI tools",
	Long: `Using kb from scripts and AI tools

Add --json to a listing or a change to get JSON instead of a table. An
empty list prints [].

  kb notes --json
  kb boards --json
  kb cards -B study --json
  kb card add "Fix login" -B study -p urgent --json
  kb note backlinks "petty cash" --json

The fields
  note       id, title, slug, path, body, tags (list), pinned, workspace,
             workspace_id, created_at, updated_at
  board      id (its vault path), name, description, workspace
  card       id, board, column, title, description, priority, labels
             (list), external_id, archived
  column     name, position, wip_limit, cards
  workspace  id, name, kind, description, path, position

With jq:
  kb cards -B study --json | jq -r '.[] | select(.priority == "urgent") | .title'

Deleting asks a question on stderr and reads the answer from stdin; with no
answer (a script) it stops with an error, so scripts pass --force (-f):

  kb card delete <id> -B study -f

Errors go to stderr, and kb exits with status 1.`,
}

func init() {
	for _, g := range []*cobra.Command{startGuide, linksGuide, kanbanGuide, tuiGuide, filesGuide, scriptingGuide} {
		rootCmd.AddCommand(g)
	}

	noteCmd.Example = `  kb notes
  kb notes --tag far
  kb notes --search "bank reconciliation"`
	noteCreateCmd.Example = `  kb note create "Petty Cash"
  kb note create "Petty Cash" --tags far,assets --body "Part of [[Cash and Cash Equivalents]]."
  kb note create "Audit Planning" -w Work`
	noteShowCmd.Example = `  kb note show "petty cash"
  kb note show petty-cash --json`
	noteEditCmd.Example = `  kb note edit "petty cash" --tags far,cash
  kb note edit "petty cash" --body "Kept for small expenses. See [[Bank Reconciliation]]."
  kb note edit "petty cash" --title "Petty Cash Fund"`
	noteRenameCmd.Example = `  kb note rename "petty cash" "Petty Cash Fund"`
	noteDeleteCmd.Example = `  kb note delete "petty cash"
  kb note delete "petty cash" -f`
	noteBacklinksCmd.Example = `  kb note backlinks "cash and cash equivalents"`
	noteSearchCmd.Example = `  kb note search bank reconciliation
  kb note search "petty cash" --json`
	noteMoveCmd.Example = `  kb note move "petty cash" -w Work
  kb note move "petty cash"            (back to Default)`
	openCmd.Example = `  kb open "petty cash"
  kb open petty-cash`
	dailyCmd.Example = `  kb daily
  kb daily --date 2026-10-01`
	tagsCmd.Example = `  kb tags
  kb tags --json`

	boardCmd.Example = `  kb boards
  kb boards --json`
	boardCreateCmd.Example = `  kb board create study
  kb board create study -d "Exam prep" -w Work`
	boardMoveCmd.Example = `  kb board move study -w Work`
	boardDeleteCmd.Example = `  kb board delete study`

	cardCmd.Example = `  kb cards -B study
  kb cards -B study -p high -l far
  kb cards -B study -c Todo -s review`
	cardAddCmd.Example = `  kb card add "Review FAR 01" -B study
  kb card add "Review FAR 01" -B study -c Todo -p high -l "far, review" -d "Chapters 1 to 3"`
	cardShowCmd.Example = `  kb card show 8899 -B study`
	cardEditCmd.Example = `  kb card edit 8899 -B study -t "Review FAR 01 to 03" -p urgent
  kb card edit 8899 -B study -l ""          (clear the labels)`
	cardMoveCmd.Example = `  kb card move 8899 "In Progress" -B study
  kb card move 8899 Todo --before 3c05 -B study`
	cardArchiveCmd.Example = `  kb card archive 8899 -B study`
	cardDeleteCmd.Example = `  kb card delete 8899 -B study`

	columnCmd.Example = `  kb columns -B study`
	columnAddCmd.Example = `  kb column add QA -B study`
	columnRenameCmd.Example = `  kb column rename QA Testing -B study`
	columnReorderCmd.Example = `  kb column reorder "Backlog,Todo,In Progress,Review,Done" -B study`
	columnWIPLimitCmd.Example = `  kb column wip-limit "In Progress" 2 -B study
  kb column wip-limit "In Progress" 0 -B study   (clear it)`
	columnDeleteCmd.Example = `  kb column delete QA -B study`

	workspaceCmd.Example = `  kb workspace
  kb workspace -k project`
	workspaceCreateCmd.Example = `  kb workspace create Work -k project -d "Day job"`
	workspaceShowCmd.Example = `  kb workspace show Work`
	workspaceEditCmd.Example = `  kb workspace edit Work --name Office -k area`
	workspaceArchiveCmd.Example = `  kb workspace archive Work`
	workspaceDeleteCmd.Example = `  kb workspace delete Work`

	graphCmd.Example = `  kb graph
  kb graph --open
  kb graph -w Work --json`
	indexCmd.Example = `  kb index
  kb index --json`
	publishCmd.Example = `  kb publish "petty cash"
  kb publish "petty cash" --draft
  kb publish "petty cash" --dry-run -t blog`
	publishSetupCmd.Example = `  kb publish setup blog --path ~/blog
  kb publish setup blog --path ~/blog --permalink "/:year/:month/:day/:title/"`
	publishListCmd.Example = `  kb publish list
  kb publish list -t blog`
	publishDeleteCmd.Example = `  kb publish delete blog`
	importCmd.Example = `  kb import --dry-run
  kb import`
}
