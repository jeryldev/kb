package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/jeryldev/kb/internal/model"
	"github.com/spf13/cobra"
)

var cardCmd = &cobra.Command{
	Use:     "cards",
	Aliases: []string{"card"},
	Short:   "Manage cards",
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		cards, err := db.ListBoardCards(board.ID)
		if err != nil {
			return err
		}

		if len(cards) == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "No cards on board %q. Add one with: kb card add \"title\"\n", board.Name)
			return nil
		}

		columns, err := db.ListColumns(board.ID)
		if err != nil {
			return err
		}
		colNames := make(map[string]string)
		for _, col := range columns {
			colNames[col.ID] = col.Name
		}

		if jsonOutput {
			out := make([]cardJSON, len(cards))
			for i, c := range cards {
				out[i] = toCardJSON(c, colNames[c.ColumnID])
			}
			return printJSON(out)
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCOLUMN\tTITLE\tPRIORITY\tLABELS")
		for _, c := range cards {
			colName := colNames[c.ColumnID]
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				c.ID[:8], colName, truncateStr(c.Title, 40), c.Priority, c.Labels)
		}
		return w.Flush()
	},
}

var cardAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a new card",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		columns, err := db.ListColumns(board.ID)
		if err != nil {
			return err
		}
		if len(columns) == 0 {
			return fmt.Errorf("board %q has no columns", board.Name)
		}

		colName, _ := cmd.Flags().GetString("column")
		priorityStr, _ := cmd.Flags().GetString("priority")

		targetCol := columns[0]
		if colName != "" {
			found := false
			for _, col := range columns {
				if strings.EqualFold(col.Name, colName) {
					targetCol = col
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("column %q not found on board %q", colName, board.Name)
			}
		}

		priority, err := model.ParsePriority(priorityStr)
		if err != nil {
			return err
		}

		card, err := db.CreateCard(targetCol.ID, args[0], priority)
		if err != nil {
			return err
		}

		needsUpdate := false
		if cmd.Flags().Changed("description") {
			card.Description, _ = cmd.Flags().GetString("description")
			needsUpdate = true
		}
		if cmd.Flags().Changed("labels") {
			card.Labels, _ = cmd.Flags().GetString("labels")
			needsUpdate = true
		}
		if cmd.Flags().Changed("external-id") {
			card.ExternalID, _ = cmd.Flags().GetString("external-id")
			needsUpdate = true
		}

		if needsUpdate {
			if err := db.UpdateCard(card); err != nil {
				return err
			}
		}

		if jsonOutput {
			return printJSON(toCardJSON(card, targetCol.Name))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Created card %q in %s (id: %s)\n", card.Title, targetCol.Name, card.ID[:8])
		return nil
	},
}

var cardEditCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a card's fields",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		cardID, err := resolveCardID(board.ID, args[0])
		if err != nil {
			return err
		}

		card, err := db.GetCard(cardID)
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("title") {
			card.Title, _ = cmd.Flags().GetString("title")
		}
		if cmd.Flags().Changed("description") {
			card.Description, _ = cmd.Flags().GetString("description")
		}
		if cmd.Flags().Changed("labels") {
			card.Labels, _ = cmd.Flags().GetString("labels")
		}
		if cmd.Flags().Changed("priority") {
			pStr, _ := cmd.Flags().GetString("priority")
			p, err := model.ParsePriority(pStr)
			if err != nil {
				return err
			}
			card.Priority = p
		}
		if cmd.Flags().Changed("external-id") {
			card.ExternalID, _ = cmd.Flags().GetString("external-id")
		}

		if err := db.UpdateCard(card); err != nil {
			return err
		}

		if jsonOutput {
			columns, err := db.ListColumns(board.ID)
			if err != nil {
				return err
			}
			colName := card.ColumnID
			for _, col := range columns {
				if col.ID == card.ColumnID {
					colName = col.Name
					break
				}
			}
			return printJSON(toCardJSON(card, colName))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Updated card %q (id: %s)\n", card.Title, card.ID[:8])
		return nil
	},
}

var cardMoveCmd = &cobra.Command{
	Use:   "move <id> <column>",
	Short: "Move a card to a different column",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		cardID, err := resolveCardID(board.ID, args[0])
		if err != nil {
			return err
		}

		targetCol, err := resolveColumnByName(board.ID, args[1])
		if err != nil {
			return err
		}

		if err := db.MoveCard(cardID, targetCol.ID); err != nil {
			return err
		}

		if jsonOutput {
			card, err := db.GetCard(cardID)
			if err != nil {
				return err
			}
			return printJSON(toCardJSON(card, targetCol.Name))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Moved card to %s\n", targetCol.Name)
		return nil
	},
}

var cardArchiveCmd = &cobra.Command{
	Use:   "archive <id>",
	Short: "Archive a card",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		cardID, err := resolveCardID(board.ID, args[0])
		if err != nil {
			return err
		}

		card, err := db.GetCard(cardID)
		if err != nil {
			return err
		}

		columns, err := db.ListColumns(board.ID)
		if err != nil {
			return err
		}
		colName := card.ColumnID
		for _, col := range columns {
			if col.ID == card.ColumnID {
				colName = col.Name
				break
			}
		}

		if err := db.ArchiveCard(cardID); err != nil {
			return err
		}

		if jsonOutput {
			card.ArchivedAt = &card.UpdatedAt
			return printJSON(toCardJSON(card, colName))
		}

		fmt.Fprintln(cmd.OutOrStdout(), "Card archived")
		return nil
	},
}

var cardDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a card",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		cardID, err := resolveCardID(board.ID, args[0])
		if err != nil {
			return err
		}

		card, err := db.GetCard(cardID)
		if err != nil {
			return err
		}

		columns, err := db.ListColumns(board.ID)
		if err != nil {
			return err
		}
		colName := card.ColumnID
		for _, col := range columns {
			if col.ID == card.ColumnID {
				colName = col.Name
				break
			}
		}

		if err := db.DeleteCard(cardID); err != nil {
			return err
		}

		if jsonOutput {
			return printJSON(toCardJSON(card, colName))
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Deleted card %q\n", card.Title)
		return nil
	},
}

var cardShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show card details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		board, err := resolveBoard()
		if err != nil {
			return err
		}

		cardID, err := resolveCardID(board.ID, args[0])
		if err != nil {
			return err
		}

		card, err := db.GetCard(cardID)
		if err != nil {
			return err
		}

		columns, err := db.ListColumns(board.ID)
		if err != nil {
			return err
		}
		colName := card.ColumnID
		for _, col := range columns {
			if col.ID == card.ColumnID {
				colName = col.Name
				break
			}
		}

		if jsonOutput {
			return printJSON(toCardJSON(card, colName))
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Title:       %s\n", card.Title)
		fmt.Fprintf(out, "Column:      %s\n", colName)
		fmt.Fprintf(out, "Priority:    %s\n", card.Priority)
		fmt.Fprintf(out, "Labels:      %s\n", card.Labels)
		if card.ExternalID != "" {
			fmt.Fprintf(out, "External ID: %s\n", card.ExternalID)
		}
		if card.Description != "" {
			fmt.Fprintf(out, "\nDescription:\n%s\n", card.Description)
		}
		fmt.Fprintf(out, "\nCreated: %s   Updated: %s\n",
			card.CreatedAt.Format("02 Jan 2006"), card.UpdatedAt.Format("02 Jan 2006"))
		fmt.Fprintf(out, "ID: %s\n", card.ID)
		return nil
	},
}

func resolveBoard() (*model.Board, error) {
	name := detectBoard()
	if name == "" {
		return nil, fmt.Errorf("cannot detect board; set KB_BOARD or use from a project directory")
	}
	board, err := db.GetBoardByName(name)
	if err != nil {
		return nil, err
	}
	if board == nil {
		return nil, fmt.Errorf("board %q not found; create it with: kb board create %s", name, name)
	}
	return board, nil
}

func resolveCardID(boardID, prefix string) (string, error) {
	cards, err := db.ListBoardCards(boardID)
	if err != nil {
		return "", err
	}

	var matches []string
	for _, c := range cards {
		if c.ID == prefix || (len(prefix) >= 4 && strings.HasPrefix(c.ID, prefix)) {
			matches = append(matches, c.ID)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no card found matching %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous card ID %q matches %d cards; use more characters", prefix, len(matches))
	}
}

func truncateStr(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

func init() {
	cardAddCmd.Flags().StringP("column", "c", "", "Target column (default: first column)")
	cardAddCmd.Flags().StringP("priority", "p", "medium", "Priority (low, medium, high, urgent)")
	cardAddCmd.Flags().StringP("description", "d", "", "Card description")
	cardAddCmd.Flags().StringP("labels", "l", "", "Comma-separated labels")
	cardAddCmd.Flags().StringP("external-id", "e", "", "External system ID")

	cardEditCmd.Flags().StringP("title", "t", "", "New title")
	cardEditCmd.Flags().StringP("description", "d", "", "New description")
	cardEditCmd.Flags().StringP("labels", "l", "", "New labels (comma-separated)")
	cardEditCmd.Flags().StringP("priority", "p", "", "New priority (low, medium, high, urgent)")
	cardEditCmd.Flags().StringP("external-id", "e", "", "New external ID")

	cardCmd.AddCommand(cardAddCmd)
	cardCmd.AddCommand(cardEditCmd)
	cardCmd.AddCommand(cardMoveCmd)
	cardCmd.AddCommand(cardArchiveCmd)
	cardCmd.AddCommand(cardDeleteCmd)
	cardCmd.AddCommand(cardShowCmd)
	rootCmd.AddCommand(cardCmd)
}
