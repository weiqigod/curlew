// Package shard splits a request list into N shards for distributed execution.
package shard

import "github.com/weiqigod/curlew/internal/parser"

// Plan is the per-shard slice plus original index map.
type Plan struct {
	// Requests is the shard's slice of RequestItems (stable order within shard).
	Requests []parser.RequestItem
	// OriginalIndex[i] is the index of Requests[i] in the pre-shard list.
	// Used by the aggregator to re-key results back to original order.
	OriginalIndex []int
}

// Split returns n shards from items using round-robin assignment
// (items[i] → shard[i%n]). Returns empty shards when items is empty;
// trailing shards are empty when len(items) < n. Panics on n < 1
// because callers must validate first.
func Split(items []parser.RequestItem, n int) []Plan {
	if n < 1 {
		panic("shard.Split: n must be >= 1")
	}

	plans := make([]Plan, n)
	for i := range plans {
		plans[i] = Plan{
			Requests:      make([]parser.RequestItem, 0),
			OriginalIndex: make([]int, 0),
		}
	}

	for i, item := range items {
		s := i % n
		plans[s].Requests = append(plans[s].Requests, item)
		plans[s].OriginalIndex = append(plans[s].OriginalIndex, i)
	}

	return plans
}
