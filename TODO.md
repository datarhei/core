# TODO

## Panics in the cluster API result in an empty 200 response

The recover middleware of the internal cluster API (`cluster/api.go`, `NewAPI`) uses a
`LogErrorFunc` that returns `nil`. Echo then doesn't write an error response, and the caller
gets an empty 200. A forwarded request that panicked on the leader looks like it succeeded
although nothing has been stored. The function should return the error such that a 500 is
sent.
