package resolver

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/datarhei/core/v16/http/graph/models"
	"github.com/datarhei/core/v16/restream"
)

// Processes is the resolver for the processes field.
func (r *queryResolver) Processes(ctx context.Context, idpattern *string, refpattern *string, domainpattern *string) ([]*models.Process, error) {
	user, _ := ctx.Value(GraphKey("user")).(string)
	ids := r.Restream.GetProcessIDs(*idpattern, *refpattern, "", *domainpattern)

	procs := []*models.Process{}

	for _, id := range ids {
		if !r.IAM.Enforce(user, id.Domain, "process:"+id.ID, "read") {
			continue
		}

		p, err := r.getProcess(id)
		if err != nil {
			return nil, err
		}

		procs = append(procs, p)
	}

	return procs, nil
}

// Process is the resolver for the process field.
func (r *queryResolver) Process(ctx context.Context, id string, domain string) (*models.Process, error) {
	user, _ := ctx.Value(GraphKey("user")).(string)

	if !r.IAM.Enforce(user, domain, "process:"+id, "read") {
		return nil, fmt.Errorf("forbidden")
	}

	tid := restream.TaskID{
		ID:     id,
		Domain: domain,
	}

	return r.getProcess(tid)
}

// Probe is the resolver for the probe field.
func (r *queryResolver) Probe(ctx context.Context, id string, domain string) (*models.Probe, error) {
	user, _ := ctx.Value(GraphKey("user")).(string)

	if !r.IAM.Enforce(user, domain, "process:"+id, "write") {
		return nil, fmt.Errorf("forbidden")
	}

	tid := restream.TaskID{
		ID:     id,
		Domain: domain,
	}

	probe := r.Restream.Probe(tid)

	p := &models.Probe{}
	p.UnmarshalRestream(probe)

	return p, nil
}
