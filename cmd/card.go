package cmd

import (
	"fmt"
	"os"
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
			fmt.Printf("No cards on board %q. Add one with: kb card add \"title\"\n", board.Name)
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

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
				if col.Name == colName {
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
		fmt.Printf("Created card %q in %s (id: %s)\n", card.Title, targetCol.Name, card.ID[:8])
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

		columns, err := db.ListColumns(board.ID)
		if err != nil {
			return err
		}

		var targetCol *model.Column
		for _, col := range columns {
			if col.Name == args[1] {
				targetCol = col
				break
			}
		}
		if targetCol == nil {
			return fmt.Errorf("column %q not found", args[1])
		}

		if err := db.MoveCard(cardID, targetCol.ID); err != nil {
			return err
		}
		fmt.Printf("Moved card to %s\n", targetCol.Name)
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

		if err := db.ArchiveCard(cardID); err != nil {
			return err
		}
		fmt.Println("Card archived")
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

		fmt.Printf("Title:       %s\n", card.Title)
		fmt.Printf("Column:      %s\n", colName)
		fmt.Printf("Priority:    %s\n", card.Priority)
		fmt.Printf("Labels:      %s\n", card.Labels)
		if card.ExternalID != "" {
			fmt.Printf("External ID: %s\n", card.ExternalID)
		}
		if card.Description != "" {
			fmt.Printf("\nDescription:\n%s\n", card.Description)
		}
		fmt.Printf("\nCreated: %s   Updated: %s\n",
			card.CreatedAt.Format("02 Jan 2006"), card.UpdatedAt.Format("02 Jan 2006"))
		fmt.Printf("ID: %s\n", card.ID)
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

	cardCmd.AddCommand(cardAddCmd)
	cardCmd.AddCommand(cardMoveCmd)
	cardCmd.AddCommand(cardArchiveCmd)
	cardCmd.AddCommand(cardShowCmd)
	rootCmd.AddCommand(cardCmd)
}
