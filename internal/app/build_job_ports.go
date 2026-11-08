package app

import (
	"context"
	"io"
	"time"

	"github.com/rbretschneider/ownership_board/internal/domain"
)

type BuildJobRepository interface {
	QueuedForHost(context.Context, domain.BuildHost, int) ([]domain.BuildJob, error)
	RunnerAssignments(context.Context) ([]RunnerAssignment, error)
	QueuedCountsByOS(context.Context) (map[domain.BuildHostOS]int, error)
	Get(context.Context, string) (domain.BuildJob, error)
	ActiveAssignments(context.Context) ([]domain.BuildJob, error)
	AssignmentCount(context.Context, int64) (int, error)
	Candidate(context.Context, domain.BuildHost) (*domain.BuildJob, error)
	CommitState(context.Context, int64, domain.BuildJobTarget, string) (known bool, latest string, err error)
	Supersede(context.Context, int64, domain.BuildJobTarget, string, time.Time) error
	Bundles(context.Context) ([]BuildJobBundle, error)
	Insert(context.Context, domain.BuildJob) error
	Save(context.Context, domain.BuildJob) error
	Assign(context.Context, domain.BuildJob) error
	Uploads(context.Context) ([]BuildJobUpload, error)
	Reserve(context.Context, BuildJobUpload) error
	FinishUpload(context.Context, string) error
}
type BuildJobBundle struct {
	ID     string
	Bytes  int64
	SHA256 string
}
type BuildJobUpload struct {
	Token     string
	JobID     string
	Metadata  domain.BuildJobSubmission
	CreatedAt time.Time
}

// BundleStorage stores immutable payloads and retained partial transfers.
// PayloadSizes includes every managed payload, including abandoned files.
type BundleStorage interface {
	// TryLockUpload returns nil when another process holds this upload's lock.
	// The caller holds the lock from before reservation until publication/abandonment.
	TryLockUpload(token string) (io.Closer, error)
	PayloadSizes() (map[string]int64, error)
	Stage(token string, metadata domain.BuildJobSubmission, src io.Reader) error
	Promote(token string, metadata domain.BuildJobSubmission) error
	Open(id string) (io.ReadCloser, error)
	Verify(id string, size int64, digest string) error
}
