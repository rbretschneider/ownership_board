package app

import (
	"context"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type BuildHostRepository interface {
	List(context.Context) ([]domain.BuildHost, error)
	Get(context.Context, int64) (domain.BuildHost, error)
	Insert(context.Context, *domain.BuildHost) error
	Approve(context.Context, domain.BuildHost) error
	Update(context.Context, domain.BuildHost) error
	Revoke(context.Context, int64, time.Time) error
}

type BuildHostPairingCodeRepository interface {
	Insert(context.Context, domain.BuildHostPairingCode) error
}
