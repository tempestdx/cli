package messages

import "testing"

func TestFormatShowingSummary(t *testing.T) {
	tests := []struct {
		name         string
		itemCount    int
		totalFetched int
		pageCount    int
		itemType     string
		hasLimit     bool
		expected     string
	}{
		{
			name:         "no limit, single page",
			itemCount:    10,
			totalFetched: 10,
			pageCount:    1,
			itemType:     "recipe",
			hasLimit:     false,
			expected:     "Showing 10 recipes from 1 page",
		},
		{
			name:         "limit, single form",
			itemCount:    1,
			totalFetched: 10,
			pageCount:    1,
			itemType:     "recipe",
			hasLimit:     true,
			expected:     "Showing 1/10 recipe",
		},
		{
			name:         "no limit, multiple pages",
			itemCount:    10,
			totalFetched: 20,
			pageCount:    2,
			itemType:     "recipe",
			hasLimit:     false,
			expected:     "Showing 10 recipes from 2 pages",
		},
		{
			name:         "limit, plural form",
			itemCount:    10,
			totalFetched: 20,
			pageCount:    2,
			itemType:     "recipe",
			hasLimit:     true,
			expected:     "Showing 10/20 recipes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatShowingSummary(tt.itemCount, tt.totalFetched, tt.pageCount, tt.itemType, tt.hasLimit)
			if got != tt.expected {
				t.Errorf("FormatShowingSummary(%d, %d, %d, %q, %t) = %q, want %q",
					tt.itemCount, tt.totalFetched, tt.pageCount, tt.itemType, tt.hasLimit, got, tt.expected)
			}
		})
	}
}
