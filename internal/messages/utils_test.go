package messages

import "testing"

func TestFormatShowingSummary(t *testing.T) {
	tests := []struct {
		name         string
		itemCount    int
		totalFetched int
		itemType     string
		hasLimit     bool
		expected     string
	}{
		{
			name:         "limit, single form",
			itemCount:    1,
			totalFetched: 10,
			itemType:     "recipe",
			hasLimit:     true,
			expected:     "Showing 1/10 recipe",
		},
		{
			name:         "limit, plural form",
			itemCount:    10,
			totalFetched: 20,
			itemType:     "project",
			hasLimit:     true,
			expected:     "Showing 10/20 projects",
		},
		{
			name:         "no limit, single form",
			itemCount:    1,
			totalFetched: 10,
			itemType:     "resource",
			hasLimit:     false,
			expected:     "Showing 1 resource",
		},
		{
			name:         "no limit, plural form",
			itemCount:    10,
			totalFetched: 20,
			itemType:     "recipe",
			hasLimit:     false,
			expected:     "Showing 10 recipes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatShowingSummary(tt.itemCount, tt.totalFetched, tt.itemType, tt.hasLimit)
			if got != tt.expected {
				t.Errorf("FormatShowingSummary(%d, %d, %q, %t) = %q, want %q",
					tt.itemCount, tt.totalFetched, tt.itemType, tt.hasLimit, got, tt.expected)
			}
		})
	}
}
