package app

import (
	"context"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

// placement says where a ticket goes within a lane.
type placement struct {
	// AfterID puts the ticket directly below that ticket; nil means the top.
	AfterID *int64
	// IsBottom puts the ticket at the end of the lane and ignores AfterID.
	IsBottom bool
}

var (
	atTop    = placement{}
	atBottom = placement{IsBottom: true}
)

// rankFor returns the rank that puts ticketID at the requested place in lane,
// renumbering the lane once if the neighbours are too close together.
func rankFor(ctx context.Context, tickets TicketRepository, lane domain.Lane, ticketID int64, place placement) (float64, error) {
	for attempt := 0; attempt < 2; attempt++ {
		order, err := laneOrderWithout(ctx, tickets, lane, ticketID)
		if err != nil {
			return 0, err
		}
		prev, next, err := neighbours(order, place)
		if err != nil {
			return 0, err
		}
		rank, needsRenumber := domain.RankBetween(prev, next)
		if !needsRenumber {
			return rank, nil
		}
		if err := renumber(ctx, tickets, order); err != nil {
			return 0, err
		}
	}
	return 0, domain.Conflictf("could not find room in lane %s", lane)
}

func laneOrderWithout(ctx context.Context, tickets TicketRepository, lane domain.Lane, ticketID int64) ([]RankedTicket, error) {
	order, err := tickets.LaneOrder(ctx, lane)
	if err != nil {
		return nil, err
	}
	kept := order[:0]
	for _, ranked := range order {
		if ranked.ID != ticketID {
			kept = append(kept, ranked)
		}
	}
	return kept, nil
}

func neighbours(order []RankedTicket, place placement) (*float64, *float64, error) {
	if place.IsBottom {
		return lastRank(order), nil, nil
	}
	if place.AfterID == nil {
		return nil, firstRank(order), nil
	}
	for i := range order {
		if order[i].ID != *place.AfterID {
			continue
		}
		prev := order[i].Rank
		if i+1 < len(order) {
			return &prev, &order[i+1].Rank, nil
		}
		return &prev, nil, nil
	}
	return nil, nil, domain.Invalidf("after_id %d is not in the target lane", *place.AfterID)
}

func firstRank(order []RankedTicket) *float64 {
	if len(order) == 0 {
		return nil
	}
	return &order[0].Rank
}

func lastRank(order []RankedTicket) *float64 {
	if len(order) == 0 {
		return nil
	}
	return &order[len(order)-1].Rank
}

func renumber(ctx context.Context, tickets TicketRepository, order []RankedTicket) error {
	for i := range order {
		order[i].Rank = domain.RenumberedRank(i)
		if err := tickets.SetRank(ctx, order[i].ID, order[i].Rank); err != nil {
			return err
		}
	}
	return nil
}
