package messages

import "testing"

func TestFormatShowingSummary(t *testing.T) {
	tests := []struct {
		name         string
		itemCount    int
		totalFetched int
		itemType     string
		expected     string
	}{
		{
			name:         "limit, single form",
			itemCount:    1,
			totalFetched: 10,
			itemType:     "recipe",
			expected:     "Showing 1/10 recipe",
		},
		{
			name:         "limit, plural form",
			itemCount:    10,
			totalFetched: 20,
			itemType:     "project",
			expected:     "Showing 10/20 projects",
		},
		{
			name:         "no limit, single form",
			itemCount:    1,
			totalFetched: 10,
			itemType:     "resource",
			expected:     "Showing 1 resource",
		},
		{
			name:         "no limit, plural form",
			itemCount:    10,
			totalFetched: 20,
			itemType:     "recipe",
			expected:     "Showing 10 recipes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatShowingSummary(tt.itemCount, tt.totalFetched, tt.itemType)
			if got != tt.expected {
				t.Errorf("FormatShowingSummary(%d, %d, %q) = %q, want %q",
					tt.itemCount, tt.totalFetched, tt.itemType, got, tt.expected)
			}
		})
	}
}
