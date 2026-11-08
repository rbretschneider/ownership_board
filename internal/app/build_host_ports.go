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
	RecordHeartbeat(context.Context, int64, domain.BuildHostReport, time.Time) error
}

type BuildHostPairingCodeRepository interface {
	Insert(context.Context, domain.BuildHostPairingCode) error
	GetByHash(context.Context, string) (domain.BuildHostPairingCode, error)
	Consume(context.Context, string, int64, time.Time) error
}

type BuildHostEventRepository interface {
	RecentForHost(context.Context, int64, int) ([]domain.BuildHostEvent, error)
	Append(context.Context, *domain.BuildHostEvent) error
	ForHost(context.Context, int64, int64, int) ([]domain.BuildHostEvent, error)
}
