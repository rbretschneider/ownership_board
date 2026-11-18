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
	// SupersedeCompleted ends queued and running jobs of tickets whose work is complete.
	SupersedeCompleted(context.Context, time.Time) error
	Supersede(context.Context, int64, domain.BuildJobTarget, string, time.Time) error
	Bundles(context.Context) ([]BuildJobBundle, error)
	Insert(context.Context, domain.BuildJob) error
	Save(context.Context, domain.BuildJob) error
	Assign(context.Context, domain.BuildJob) error
	Uploads(context.Context) ([]BuildJobUpload, error)
	Reserve(context.Context, BuildJobUpload) error
	FinishUpload(context.Context, string) error
	// RecentForTicket returns the ticket's newest jobs first, at most limit.
	RecentForTicket(ctx context.Context, ticketID int64, limit int) ([]domain.BuildJob, error)
	// Open returns every queued or running job.
	Open(ctx context.Context) ([]domain.BuildJob, error)
	// SetBatch holds a queued job for a batch (batchID set) or frees it (empty);
	// solo marks it to be verified alone from now on.
	SetBatch(ctx context.Context, jobID, batchID string, solo bool) error
	// BatchCandidates returns free queued jobs that can share a batch with lead, oldest first.
	BatchCandidates(ctx context.Context, lead domain.BuildJob, limit int) ([]domain.BuildJob, error)
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

// BuildBatchRepository persists batched verification runs.
type BuildBatchRepository interface {
	Get(context.Context, string) (domain.BuildBatch, error)
	// Insert stores the batch and holds every member job for it.
	Insert(context.Context, domain.BuildBatch) error
	// Save writes the run state and each member's outcome.
	Save(context.Context, domain.BuildBatch) error
	// QueuedForHost returns queued batches (bisection halves) the host can run, oldest first.
	QueuedForHost(context.Context, domain.BuildHost) ([]domain.BuildBatch, error)
	ActiveAssignments(context.Context) ([]domain.BuildBatch, error)
	AssignmentCount(context.Context, int64) (int, error)
	// ForTicket returns the batches that held any of the ticket's jobs, newest first.
	ForTicket(ctx context.Context, ticketID int64, limit int) ([]domain.BuildBatch, error)
}
