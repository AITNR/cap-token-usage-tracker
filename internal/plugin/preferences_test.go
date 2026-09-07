package plugin

import "testing"

func TestDashboardPreferencesAllowCacheHitRateColumn(t *testing.T) {
	preferences, err := normalizeDashboardPreferences(DashboardPreferences{
		RequestPageSize:        10,
		DimensionPageSize:      10,
		HiddenRequestColumns:   []string{"cache_hit_rate"},
		HiddenDimensionColumns: []string{},
		TimeRangeMode:          "custom",
	})
	if err != nil {
		t.Fatalf("cache hit rate column rejected by preferences: %v", err)
	}
	if len(preferences.HiddenRequestColumns) != 1 || preferences.HiddenRequestColumns[0] != "cache_hit_rate" {
		t.Fatalf("cache hit rate column was not retained: %+v", preferences)
	}
}
