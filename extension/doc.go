// Package extension provides useful extensions to the memstruct constructs.
//
// DenseLocalGrid and DenseLocalGridPool:
// Use DenseLocalGrid (DenseLocalGridCreateFull / DenseLocalGridCreateSparse) when you have
// a single grid or a small number of independent grids; each grid owns its allocations.
// Use DenseLocalGridPool (DenseLocalGridPoolBuild) when you have many logical grids (e.g.
// one per state in a DPDA) that should share a single set of allocations: one global pool
// per keyspace and value/bitmap region, with per-grid descriptors (offsets and lengths).
// Pooled grids avoid per-state allocator overhead and fragmentation while keeping per-state
// logical locality.
package extension
