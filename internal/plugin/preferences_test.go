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

func TestDashboardPreferencesNormalizeTokenDisplayMode(t *testing.T) {
	for _, mode := range []string{"full", "k", "m", "B"} {
		preferences, err := normalizeDashboardPreferences(DashboardPreferences{
			RequestPageSize:   10,
			DimensionPageSize: 10,
			TimeRangeMode:     "custom",
			TokenDisplayMode:  mode,
		})
		if err != nil || preferences.TokenDisplayMode != mode {
			t.Fatalf("token display mode %q: preferences=%+v err=%v", mode, preferences, err)
		}
	}

	empty := defaultDashboardPreferences()
	if _, err := normalizeDashboardPreferences(empty); err != nil || empty.TokenDisplayMode != "full" {
		t.Fatalf("empty token display mode: preferences=%+v err=%v", empty, err)
	}

	invalid := defaultDashboardPreferences()
	invalid.TokenDisplayMode = "billion"
	if _, err := normalizeDashboardPreferences(invalid); err == nil {
		t.Fatal("unsupported token display mode was accepted")
	}
}
