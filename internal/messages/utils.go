package messages

import (
	"fmt"
	"github.com/dustin/go-humanize/english"
)

// FormatShowingSummary returns a formatted summary string for showing items
// If limitFlag > 0, returns "Showing X/Y items"
// Otherwise returns "Showing X items from Y pages"
func FormatShowingSummary(itemCount, totalFetched, pageCount int, itemType string, hasLimit bool) string {
	if hasLimit {
		return fmt.Sprintf("Showing %d/%d %s", itemCount, totalFetched, english.PluralWord(itemCount, itemType, ""))
	}
	return fmt.Sprintf("Showing %d %s from %d %s",
		itemCount, english.PluralWord(itemCount, itemType, ""),
		pageCount, english.PluralWord(pageCount, "page", ""))
}
