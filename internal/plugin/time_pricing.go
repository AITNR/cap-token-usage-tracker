package plugin

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	_ "time/tzdata" // Keep named zones available in Windows and minimal containers.
)

const maxTimePriceTiers = 32

// TimePriceTier repeats weekly. Days use ISO weekdays; empty means every day.
// Overnight windows belong to their starting day. End is exclusive.
type TimePriceTier struct {
	Name  string `json:"name"`
	Days  []int  `json:"days,omitempty"`
	Start string `json:"start"`
	End   string `json:"end"`
	TokenRates
}

func clockMinute(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil || parsed.Format("15:04") != value {
		return 0, fmt.Errorf("invalid time %q: use HH:mm", value)
	}
	return parsed.Hour()*60 + parsed.Minute(), nil
}

func timeTierDays(tier TimePriceTier) []int {
	if len(tier.Days) == 0 {
		return []int{1, 2, 3, 4, 5, 6, 7}
	}
	return tier.Days
}

func normalizeTimePricing(model string, price *ModelPrice) error {
	price.TimeZone = strings.TrimSpace(price.TimeZone)
	if len(price.TimeTiers) > maxTimePriceTiers {
		return fmt.Errorf("model %q: at most %d time tiers", model, maxTimePriceTiers)
	}
	if price.TimeZone == "" && len(price.TimeTiers) > 0 {
		price.TimeZone = "UTC"
	}
	if price.TimeZone != "" {
		if price.TimeZone == "Local" {
			return fmt.Errorf("model %q: use an explicit time zone", model)
		}
		if _, err := time.LoadLocation(price.TimeZone); err != nil {
			return fmt.Errorf("model %q: invalid time zone %q", model, price.TimeZone)
		}
	}
	price.TimeTiers = cloneTimeTiers(price.TimeTiers)
	var occupied [7 * 1440]bool
	names := map[string]bool{}
	for i := range price.TimeTiers {
		tier := &price.TimeTiers[i]
		tier.Name = strings.TrimSpace(tier.Name)
		if tier.Name == "" || !validOptionalDimension(tier.Name) || names[tier.Name] {
			return fmt.Errorf("model %q: time tier names must be nonempty and unique", model)
		}
		names[tier.Name] = true
		start, err := clockMinute(tier.Start)
		if err != nil {
			return err
		}
		end, err := clockMinute(tier.End)
		if err != nil {
			return err
		}
		if start == end {
			return fmt.Errorf("model %q: time tier start and end must differ", model)
		}
		if err := validateTokenRates(tier.TokenRates, model, "time tier "+tier.Name); err != nil {
			return err
		}
		sort.Ints(tier.Days)
		for j, day := range tier.Days {
			if day < 1 || day > 7 || (j > 0 && tier.Days[j-1] == day) {
				return fmt.Errorf("model %q: days must be unique integers from 1 to 7", model)
			}
		}
		duration := (end - start + 1440) % 1440
		for _, day := range timeTierDays(*tier) {
			for minute := 0; minute < duration; minute++ {
				index := ((day-1)*1440 + start + minute) % len(occupied)
				if occupied[index] {
					return fmt.Errorf("model %q: overlapping time tiers", model)
				}
				occupied[index] = true
			}
		}
	}
	return nil
}

func cloneTimeTiers(tiers []TimePriceTier) []TimePriceTier {
	result := append([]TimePriceTier(nil), tiers...)
	for i := range result {
		result[i].Days = append([]int(nil), result[i].Days...)
	}
	return result
}
func sameTimePricing(a, b ModelPrice) bool {
	return a.TimeZone == b.TimeZone && reflect.DeepEqual(a.TimeTiers, b.TimeTiers)
}

// Match wall-clock time in the configured zone, including repeated DST hours.
func selectTimePriceTier(price ModelPrice, requestedAt time.Time) (TimePriceTier, bool) {
	if len(price.TimeTiers) == 0 || requestedAt.IsZero() {
		return TimePriceTier{}, false
	}
	zone := price.TimeZone
	if zone == "" {
		zone = "UTC"
	}
	if zone == "Local" {
		return TimePriceTier{}, false
	}
	location := price.timeLocation
	var err error
	if location == nil {
		location, err = time.LoadLocation(zone)
	}
	if err != nil {
		return TimePriceTier{}, false
	}
	local := requestedAt.In(location)
	day := (int(local.Weekday())+6)%7 + 1
	minute := local.Hour()*60 + local.Minute()
	var matched TimePriceTier
	found := false
	for _, tier := range price.TimeTiers {
		start, e1 := clockMinute(tier.Start)
		end, e2 := clockMinute(tier.End)
		if e1 != nil || e2 != nil || start == end {
			return TimePriceTier{}, false
		}
		matchDay := day
		hit := minute >= start && minute < end
		if start > end {
			hit = minute >= start || minute < end
			if minute < end {
				matchDay = (day+5)%7 + 1
			}
		}
		if !hit {
			continue
		}
		for _, allowed := range timeTierDays(tier) {
			if allowed == matchDay {
				if found {
					return TimePriceTier{}, false
				}
				matched = tier
				found = true
				break
			}
		}
	}
	return matched, found
}
