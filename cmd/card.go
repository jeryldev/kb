package cmd

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/jeryldev/kb/internal/fstore"
	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

var cardCmd = &cobra.Command{
	Use:     "cards",
	Aliases: []string{"card"},
	Short:   "Manage the current board's cards",
	Long: `Manage the current board's cards (see kb --help for which board that is).

A card is named by its id, the ^id at the end of its line in the board's
file, or by the first 4 or more characters of it. A card that is not on
the current board is looked for on every board, unless -B names the
board to look on.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		filter, err := cardFilterFlags(cmd)
		if err != nil {
			return err
		}
		all, err := db.BoardCards(board.ID)
		if err != nil {
			return err
		}
		var cards []*model.Card
		for _, c := range all {
			if filter.match(c) {
				cards = append(cards, c)
			}
		}
		if jsonOutput {
			return jsonList(cards, toCardJSON)
		}
		if len(cards) == 0 {
			if filter != (cardFilter{}) {
				fmt.Fprintln(cmd.OutOrStdout(), "No cards match the given filters.")
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "No cards on board %q. Add one with: kb card add \"title\"\n", board.Name)
			}
			return nil
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCOLUMN\tTITLE\tPRIORITY\tLABELS")
		for _, c := range cards {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", c.ID, c.ColumnID, truncateStr(c.Title, 40), c.Priority, strings.Join(c.LabelList(), ", "))
		}
		return w.Flush()
	},
}

type cardFilter struct {
	priority model.Priority
	label    string
	column   string
	search   string
}

func cardFilterFlags(cmd *cobra.Command) (cardFilter, error) {
	var f cardFilter
	if cmd.Flags().Changed("priority") {
		s, _ := cmd.Flags().GetString("priority")
		p, err := model.ParsePriority(s)
		if err != nil {
			return f, err
		}
		f.priority = p
	}
	f.label, _ = cmd.Flags().GetString("label")
	f.column, _ = cmd.Flags().GetString("column")
	f.search, _ = cmd.Flags().GetString("search")
	return f, nil
}

func (f cardFilter) match(c *model.Card) bool {
	if f.priority != "" && c.Priority != f.priority {
		return false
	}
	if f.label != "" && !c.HasLabel(f.label) {
		return false
	}
	if f.column != "" && !strings.EqualFold(c.ColumnID, f.column) {
		return false
	}
	if f.search != "" {
		text := strings.ToLower(c.Title + "\n" + c.Description)
		for _, word := range strings.Fields(strings.ToLower(f.search)) {
			if !strings.Contains(text, word) {
				return false
			}
		}
	}
	return true
}

func splitLabels(s string) []string {
	var out []string
	for _, l := range strings.Split(s, ",") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

var cardAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a card to the current board",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := currentBoard()
		if err != nil {
			return err
		}
		lanes, err := db.Lanes(board.ID)
		if err != nil {
			return err
		}
		if len(lanes) == 0 {
			return fmt.Errorf("board %q has no columns; add one with: kb column add <name>", board.Name)
		}
		lane := lanes[0]
		if name, _ := cmd.Flags().GetString("column"); name != "" {
			if lane, err = resolveLane(board.ID, name); err != nil {
				return err
			}
		}
		var fields fstore.CardFields
		pStr, _ := cmd.Flags().GetString("priority")
		priority, err := model.ParsePriority(pStr)
		if err != nil {
			return err
		}
		fields.Priority = string(priority)
		fields.Description, _ = cmd.Flags().GetString("description")
		labels, _ := cmd.Flags().GetString("labels")
		fields.Labels = splitLabels(labels)
		fields.ExternalID, _ = cmd.Flags().GetString("external-id")
		force, _ := cmd.Flags().GetBool("force")

		card, err := db.AddCard(board.ID, lane.Name, args[0], fields, force)
		if err != nil {
			return withForceHint(err)
		}
		if jsonOutput {
			return printJSON(toCardJSON(card))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Created card %q in %s (id: %s)\n", card.Title, card.ColumnID, card.ID)
		return nil
	},
}

var cardEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Change a card's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		card, err := resolveCard(args[0])
		if err != nil {
			return err
		}
		flags := cmd.Flags()
		if !changedAny(cmd, "title", "description", "labels", "priority", "external-id") {
			return fmt.Errorf("nothing to change; give at least one of --title, --description, --priority, --labels or --external-id")
		}
		if flags.Changed("title") {
			card.Title, _ = flags.GetString("title")
		}
		if flags.Changed("description") {
			card.Description, _ = flags.GetString("description")
		}
		if flags.Changed("labels") {
			card.Labels, _ = flags.GetString("labels")
		}
		if flags.Changed("priority") {
			s, _ := flags.GetString("priority")
			if card.Priority, err = model.ParsePriority(s); err != nil {
				return err
			}
		}
		if flags.Changed("external-id") {
			card.ExternalID, _ = flags.GetString("external-id")
		}
		if err := db.UpdateCard(card); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toCardJSON(card))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated card %q (id: %s)\n", card.Title, card.ID)
		return nil
	},
}

var cardMoveCmd = &cobra.Command{
	Use:   "move <id> <column>",
	Short: "Move a card to a column (to the end, or above --before)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		card, err := resolveCard(args[0])
		if err != nil {
			return err
		}
		lane, err := resolveLane(card.BoardID, args[1])
		if err != nil {
			return err
		}
		before := ""
		if ref, _ := cmd.Flags().GetString("before"); ref != "" {
			other, err := db.FindCard(card.BoardID, ref)
			if err != nil {
				return err
			}
			before = other.ID
		}
		force, _ := cmd.Flags().GetBool("force")
		if err := db.MoveCard(card.BoardID, card.ID, lane.Name, before, force); err != nil {
			return withForceHint(err)
		}
		if card, err = db.GetCard(card.BoardID, card.ID); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toCardJSON(card))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Moved card %q to %s\n", card.Title, lane.Name)
		return nil
	},
}

var cardArchiveCmd = &cobra.Command{
	Use:   "archive <id>",
	Short: "Archive a card",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		card, err := resolveCard(args[0])
		if err != nil {
			return err
		}
		if err := db.ArchiveCard(card.BoardID, card.ID); err != nil {
			return err
		}
		if card, err = db.GetCard(card.BoardID, card.ID); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toCardJSON(card))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Archived card %q\n", card.Title)
		return nil
	},
}

var cardDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a card from its board",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		card, err := resolveCard(args[0])
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		// The card may be on another board than the current one (see
		// resolveCard), so the question names it.
		boardName := card.BoardID
		if b, err := db.GetBoard(card.BoardID); err == nil {
			boardName = b.Name
		}
		if err := confirm(cmd, force, fmt.Sprintf("Delete card %q on board %q? (kb card archive keeps it)", card.Title, boardName)); err != nil {
			return err
		}
		if err := db.DeleteCard(card.BoardID, card.ID); err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toCardJSON(card))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted card %q\n", card.Title)
		return nil
	},
}

var cardShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a card",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		card, err := resolveCard(args[0])
		if err != nil {
			return err
		}
		if jsonOutput {
			return printJSON(toCardJSON(card))
		}
		out := cmd.OutOrStdout()
		column := card.ColumnID
		if card.ArchivedAt != nil {
			column += " (archived)"
		}
		fmt.Fprintf(out, "Title:       %s\n", card.Title)
		fmt.Fprintf(out, "Board:       %s\n", boardNameOf(card.BoardID))
		fmt.Fprintf(out, "Column:      %s\n", column)
		fmt.Fprintf(out, "Priority:    %s\n", card.Priority)
		if labels := card.LabelList(); len(labels) > 0 {
			fmt.Fprintf(out, "Labels:      %s\n", strings.Join(labels, ", "))
		}
		if card.ExternalID != "" {
			fmt.Fprintf(out, "External ID: %s\n", card.ExternalID)
		}
		if card.Description != "" {
			fmt.Fprintf(out, "\n%s\n", card.Description)
		}
		fmt.Fprintf(out, "\nID: %s\n", card.ID)
		return nil
	},
}

func init() {
	cardCmd.Flags().StringP("priority", "p", "", "Only cards with this priority (low, medium, high, urgent)")
	cardCmd.Flags().StringP("label", "l", "", "Only cards with this label")
	cardCmd.Flags().StringP("column", "c", "", "Only cards in this column")
	cardCmd.Flags().StringP("search", "s", "", "Only cards whose title or description has all these words")

	cardAddCmd.Flags().StringP("column", "c", "", "Column (default: the first)")
	cardAddCmd.Flags().StringP("priority", "p", "medium", "Priority (low, medium, high, urgent)")
	cardAddCmd.Flags().StringP("description", "d", "", "Description")
	cardAddCmd.Flags().StringP("labels", "l", "", "Comma-separated labels (kept as #tags)")
	cardAddCmd.Flags().StringP("external-id", "e", "", "ID in another system")
	cardAddCmd.Flags().BoolP("force", "f", false, "Add even if the column is at its WIP limit")

	cardEditCmd.Flags().StringP("title", "t", "", "New title")
	cardEditCmd.Flags().StringP("description", "d", "", "New description")
	cardEditCmd.Flags().StringP("labels", "l", "", "New labels, comma-separated (\"\" clears them)")
	cardEditCmd.Flags().StringP("priority", "p", "", "New priority (low, medium, high, urgent)")
	cardEditCmd.Flags().StringP("external-id", "e", "", "New external ID")

	cardMoveCmd.Flags().String("before", "", "Put the card above this card (one in the target column)")
	cardMoveCmd.Flags().BoolP("force", "f", false, "Move even if the column is at its WIP limit")

	cardDeleteCmd.Flags().BoolP("force", "f", false, "Skip confirmation")

	cardCmd.AddCommand(cardAddCmd, cardEditCmd, cardMoveCmd, cardArchiveCmd, cardDeleteCmd, cardShowCmd)
	rootCmd.AddCommand(cardCmd)
}

// changedAny reports whether any of the named flags was given.
func changedAny(cmd *cobra.Command, names ...string) bool {
	for _, n := range names {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}

// withForceHint says how to go over a column's card limit from the
// command line.
func withForceHint(err error) error {
	var wip *fstore.WIPLimitError
	if errors.As(err, &wip) {
		return fmt.Errorf("%w (use --force to go over it)", err)
	}
	return err
}
