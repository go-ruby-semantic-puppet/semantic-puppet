// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"fmt"
	"testing"
)

// benchUniverse builds a representative, resolvable dependency universe: width
// modules, each published at depth versions, each depending on the next two
// modules in the chain with an open lower-bounded range. The staggered ranges
// force the resolver to backtrack from the newest candidates.
//
// A fresh universe is built per call because Query populates each release's
// dependency lists in place; reusing a source across queries would accumulate
// state.
func benchUniverse(width, depth int) *fakeSource {
	src := newFakeSource()
	for i := 0; i < width; i++ {
		name := fmt.Sprintf("mod%02d", i)
		deps := map[string]string{}
		if i+1 < width {
			deps[fmt.Sprintf("mod%02d", i+1)] = ">= 1.0.0"
		}
		if i+2 < width {
			deps[fmt.Sprintf("mod%02d", i+2)] = ">= 1.0.0 < 3.0.0"
		}
		for v := 0; v < depth; v++ {
			rel, err := CreateRelease(src, name, fmt.Sprintf("%d.0.0", v+1), deps)
			if err != nil {
				panic(err)
			}
			src.releases[name] = append(src.releases[name], rel)
		}
	}
	return src
}

// BenchmarkResolve measures the solver on a pre-built graph (the graph is
// reused; Resolve never mutates it). See BENCHMARKS.md for the MRI comparison
// methodology.
func BenchmarkResolve(b *testing.B) {
	ClearSources()
	AddSource(benchUniverse(24, 5))
	graph, err := Query(map[string]string{"mod00": ">= 1.0.0"})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Resolve(graph); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkQueryResolve measures the full cold path: build the universe, fetch
// it into a graph and resolve it, per iteration.
func BenchmarkQueryResolve(b *testing.B) {
	specs := map[string]string{"mod00": ">= 1.0.0"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ClearSources()
		AddSource(benchUniverse(24, 5))
		b.StartTimer()
		graph, err := Query(specs)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := Resolve(graph); err != nil {
			b.Fatal(err)
		}
	}
}
