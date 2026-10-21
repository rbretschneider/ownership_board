package domain

import "math"

// RankStep is the gap left between neighbouring tickets, so most moves fit
// between two existing ranks without renumbering a lane.
const RankStep = 1024.0

// minRankGap is the smallest gap still worth splitting.
const minRankGap = 1e-6

// RankBetween returns a rank that sorts after prev and before next. Either may
// be nil for the start or end of a lane. The flag reports that the neighbours
// are too close and the lane should be renumbered before retrying.
func RankBetween(prev, next *float64) (float64, bool) {
	switch {
	case prev == nil && next == nil:
		return RankStep, false
	case prev == nil:
		return *next - RankStep, false
	case next == nil:
		return *prev + RankStep, false
	}
	if math.Abs(*next-*prev) < minRankGap {
		return 0, true
	}
	return *prev + (*next-*prev)/2, false
}

// RenumberedRank is the rank of the item at index after renumbering a lane.
func RenumberedRank(index int) float64 {
	return float64(index+1) * RankStep
}
