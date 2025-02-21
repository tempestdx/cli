package messages

import (
	"fmt"
	"strings"
)

// FormatShowingSummary returns a formatted summary string for showing items
// If limitFlag > 0, returns "Showing X/Y items"
// Otherwise returns "Showing X items from Y pages"
func FormatShowingSummary(itemCount, totalFetched, pageCount int, itemType string, hasLimit bool) string {
	if hasLimit {
		return fmt.Sprintf("Showing %d/%d %s", itemCount, totalFetched, pluralize(itemType, itemCount))
	}
	return fmt.Sprintf("Showing %d %s from %d %s",
		itemCount, pluralize(itemType, itemCount),
		pageCount, pluralize("page", pageCount))
}

// Pluralize returns the plural form of a word based on count.
// For count == 1, returns the original word.
// For count != 1, adds 's' or 'es' depending on the word ending.
func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}
	suffixes := []string{"s", "sh", "ch", "x", "z"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(word, suffix) {
			return word + "es"
		}
	}

	return word + "s"
}

