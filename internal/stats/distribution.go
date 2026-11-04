package stats

import (
	"math"
	"slices"
)

// p90 is the percentile reported alongside the median.
const p90 = 0.9

// Distribution summarises durations in hours. With Count zero the other
// fields are zero and mean "no data", not "instant".
type Distribution struct {
	Count       int     `json:"count"`
	MedianHours float64 `json:"median_hours"`
	P90Hours    float64 `json:"p90_hours"`
}

// distributionOf summarises hours; it does not modify its argument.
func distributionOf(hours []float64) Distribution {
	if len(hours) == 0 {
		return Distribution{}
	}
	sorted := slices.Clone(hours)
	slices.Sort(sorted)
	return Distribution{
		Count:       len(sorted),
		MedianHours: roundTenth(median(sorted)),
		P90Hours:    roundTenth(nearestRank(sorted, p90)),
	}
}

func median(sorted []float64) float64 {
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

// nearestRank is the smallest value with at least fraction of values at or
// below it.
func nearestRank(sorted []float64, fraction float64) float64 {
	rank := int(math.Ceil(fraction * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}

func roundTenth(value float64) float64 {
	return math.Round(value*10) / 10
}
