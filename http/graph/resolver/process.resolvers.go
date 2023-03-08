package resolver

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/datarhei/core/v16/http/graph/models"
)

// Processes is the resolver for the processes field.
func (r *queryResolver) Processes(ctx context.Context, idpattern *string, refpattern *string, group *string) ([]*models.Process, error) {
	user, _ := ctx.Value(GraphKey("user")).(string)
	ids := r.Restream.GetProcessIDs(*idpattern, *refpattern, user, *group)

	procs := []*models.Process{}

	for _, id := range ids {
		p, err := r.getProcess(id, user, *group)
		if err != nil {
			return nil, err
		}

		procs = append(procs, p)
	}

	return procs, nil
}

// Process is the resolver for the process field.
func (r *queryResolver) Process(ctx context.Context, id string, group *string) (*models.Process, error) {
	user, _ := ctx.Value(GraphKey("user")).(string)

	return r.getProcess(id, user, *group)
}

// Probe is the resolver for the probe field.
func (r *queryResolver) Probe(ctx context.Context, id string, group *string) (*models.Probe, error) {
	user, _ := ctx.Value(GraphKey("user")).(string)

	probe := r.Restream.Probe(id, user, *group)

	p := &models.Probe{}
	p.UnmarshalRestream(probe)

	return p, nil
}
