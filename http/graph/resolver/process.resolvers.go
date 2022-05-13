package resolver

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/datarhei/core/http/graph/models"
)

func (r *queryResolver) Processes(ctx context.Context) ([]*models.Process, error) {
	ids := r.Restream.GetProcessIDs()

	procs := []*models.Process{}

	for _, id := range ids {
		p, err := r.getProcess(id)
		if err != nil {
			return nil, err
		}

		procs = append(procs, p)
	}

	return procs, nil
}

func (r *queryResolver) Process(ctx context.Context, id string) (*models.Process, error) {
	return r.getProcess(id)
}

func (r *queryResolver) Probe(ctx context.Context, id string) (*models.Probe, error) {
	probe := r.Restream.Probe(id)

	p := &models.Probe{}
	p.UnmarshalRestream(probe)

	return p, nil
}
