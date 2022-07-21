package resolver

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/datarhei/core/v16/http/graph/graph"
)

func (r *mutationResolver) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}

func (r *queryResolver) Ping(ctx context.Context) (string, error) {
	return "pong", nil
}

// Mutation returns graph.MutationResolver implementation.
func (r *Resolver) Mutation() graph.MutationResolver { return &mutationResolver{r} }

// Query returns graph.QueryResolver implementation.
func (r *Resolver) Query() graph.QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }
type queryResolver struct{ *Resolver }
