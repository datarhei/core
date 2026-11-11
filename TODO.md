# TODO

## `GetRandomNode` panics if no node is registered

`Manager.GetRandomNode` in `cluster/node/manager.go` calls `rand.IntN(len(p.nodes))`, which
panics if there are no nodes, e.g. if a process is added right after the cluster started
("0 of 2 nodes registered"). `cluster.ProcessAdd` (`cluster/process.go`) calls it to get a
node ID for the process. It should return an error or an empty node ID instead.

## Panics in the cluster API result in an empty 200 response

The recover middleware of the internal cluster API (`cluster/api.go`, `NewAPI`) uses a
`LogErrorFunc` that returns `nil`. Echo then doesn't write an error response, and the caller
gets an empty 200. A forwarded request that panicked on the leader, e.g. the `GetRandomNode`
panic above, looks like it succeeded although nothing has been stored. The function should
return the error such that a 500 is sent.
