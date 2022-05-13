package resolver

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"time"

	"github.com/datarhei/core/app"
	"github.com/datarhei/core/http/graph/models"
	"github.com/datarhei/core/http/graph/scalars"
)

func (r *queryResolver) About(ctx context.Context) (*models.About, error) {
	createdAt := r.Restream.CreatedAt()

	about := &models.About{
		App:           app.Name,
		ID:            r.Restream.ID(),
		Name:          r.Restream.Name(),
		CreatedAt:     createdAt,
		UptimeSeconds: scalars.Uint64(time.Since(createdAt).Seconds()),
		Version: &models.AboutVersion{
			Number:           app.Version.String(),
			RepositoryCommit: app.Commit,
			RepositoryBranch: app.Branch,
			BuildDate:        app.Build,
			Arch:             app.Arch,
			Compiler:         app.Compiler,
		},
	}

	return about, nil
}
