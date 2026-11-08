package app

import (
	"context"
	"github.com/rbretschneider/ownership_board/internal/domain"
	"io"
	"time"
)

type BuildJobRepository interface {
	Get(context.Context, string) (domain.BuildJob, error)
	List(context.Context) ([]domain.BuildJob, error)
	Insert(context.Context, domain.BuildJob) error
	Save(context.Context, domain.BuildJob) error
	Assign(context.Context, domain.BuildJob) error
	Uploads(context.Context) ([]BuildJobUpload, error)
	Reserve(context.Context, BuildJobUpload) error
	FinishUpload(context.Context, string) error
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
	PayloadSizes() (map[string]int64, error)
	Stage(token string, metadata domain.BuildJobSubmission, src io.Reader) error
	Promote(token string, metadata domain.BuildJobSubmission) error
	Open(id string) (io.ReadCloser, error)
	Verify(id string, size int64, digest string) error
}
