<!--
SPDX-License-Identifier: BSD-3-Clause
Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors
-->

# Dependency-solver benchmarks

This document records the performance of the pure-Go `SemanticPuppet::Dependency`
resolver ([`Resolve`] / [`Query`]) against the reference Ruby `semantic_puppet`
gem, per the org's standing rule that a port should be *at least as fast as the
reference implementation*.

The two implementations resolve the **same** dependency universe and are
verified to produce the **byte-identical resolution** (see "Differential
validation" below), so the timings compare like for like.

## What is measured

`benchUniverse(width, depth)` in `dependency_bench_test.go` builds a
representative, resolvable universe:

* `width` modules `mod00 … mod{width-1}`, each published at `depth` versions
  (`1.0.0 … depth.0.0`);
* every module `modI` depends on `mod{I+1}` (`>= 1.0.0`) and `mod{I+2}`
  (`>= 1.0.0 < 3.0.0`).

The staggered upper bound on the second dependency forces the resolver to reject
the newest candidates of the deeper modules and **backtrack**, so the benchmark
exercises the conflict/backtracking path rather than a trivial newest-wins walk.

Two scenarios are timed:

* **resolve (warm)** — one graph is built once with `Query`; each iteration calls
  `Resolve` on it. This isolates the solver itself (`Resolve` never mutates the
  graph). It is the fairest head-to-head number.
* **query+resolve (cold)** — each iteration builds the whole universe from
  scratch (parsing every version and range via `CreateRelease`), fetches it into
  a graph with `Query`, and resolves it.

## Methodology

* Universe size: `width = 24`, `depth = 5` (24 modules × 5 versions = 120
  releases; ~46 dependency edges).
* Go: `go test -bench . -benchtime 1s` (`testing`'s own iteration control;
  `-benchmem` for allocations).
* Ruby: `Benchmark.realtime` over a fixed iteration count after a warm-up, with
  the same universe and the same top-level query (`mod00 => '>= 1.0.0'`).
  Script: `scripts/bench.rb` (reproduced at the end of this file).
* Both implementations run on the **same host**. The Go and Ruby processes use
  the identical universe generator, so the comparison is hardware-neutral and
  reproduces identically inside a Linux Tart VM (`tart run`), where the same two
  commands can be issued.

Reference host for the figures below:

| | |
|---|---|
| Machine | Apple M4 Max |
| OS / arch | `darwin/arm64` |
| Go | stable toolchain (go.mod floor `1.26.4`) |
| Ruby (MRI) | 4.0.5 `+PRISM` |
| gem | `semantic_puppet` 1.1.1 |

## Results

| Scenario | Ruby `semantic_puppet` (MRI) | Pure-Go (this lib) | Speed-up |
|---|---:|---:|---:|
| resolve (warm graph)      | 184 µs/op   | **16.4 µs/op**  | **≈ 11×** |
| query + resolve (cold)    | 2 910 µs/op | **143 µs/op**   | **≈ 20×** |

Go allocation profile (`-benchmem`):

```
BenchmarkResolve-16        16.4 µs/op    14704 B/op    131 allocs/op
BenchmarkQueryResolve-16   143  µs/op    79301 B/op   2350 allocs/op
```

The pure-Go resolver is comfortably faster than the reference on the warm solver
path, and the gap widens on the cold path because Go parses the 120
versions/ranges (`CreateRelease`) far more cheaply than MRI constructs the
equivalent `Version`/`VersionRange` objects.

> Note on repeated `Query` against an in-memory source: `Query` populates each
> release's candidate lists in place, so a source that returns the *same* release
> objects on every fetch accumulates state across queries. The cold benchmark
> therefore rebuilds a fresh universe each iteration (as does the Ruby script);
> this mirrors how a real Forge source hands back fresh releases per fetch.

## Differential validation

The Go and MRI resolvers were run on the identical `width=24, depth=5` universe
and produced the same solution (24 releases). The staggered upper bound pins the
two head modules to their newest versions and every deeper module to `2.0.0`:

```
[mod00 5.0.0] [mod01 5.0.0] [mod02 2.0.0] [mod03 2.0.0] … [mod23 2.0.0]
```

This is asserted structurally by the table-driven differential tests in
`dependency_test.go`, which mirror `semantic_puppet`'s own dependency spec
(simple resolution, transitive deps, newest-preferred and stable-over-prerelease
selection, overlapping/non-overlapping competing constraints with backtracking,
cycles, missing modules, and module- and graph-level constraints).

## Reproducing

Go:

```console
$ GOWORK=off go test -run xxx -bench 'Benchmark' -benchmem ./...
```

Ruby (`scripts/bench.rb`, requires `gem install semantic_puppet`):

```ruby
STDOUT.sync = true
require 'semantic_puppet'
require 'benchmark'

class FixedSource < SemanticPuppet::Dependency::Source
  attr_accessor :u
  def fetch(name); @u[name] || []; end
end

WIDTH = 24
DEPTH = 5

def build_universe
  s = FixedSource.new
  u = {}
  (0...WIDTH).each do |i|
    name = "mod%02d" % i
    deps = {}
    deps["mod%02d" % (i + 1)] = ">= 1.0.0"         if i + 1 < WIDTH
    deps["mod%02d" % (i + 2)] = ">= 1.0.0 < 3.0.0" if i + 2 < WIDTH
    u[name] = (1..DEPTH).map { |v| s.create_release(name, "#{v}.0.0", deps) }
  end
  s.u = u
  s
end

SemanticPuppet::Dependency.clear_sources
SemanticPuppet::Dependency.add_source(build_universe)
g = SemanticPuppet::Dependency.query('mod00' => '>= 1.0.0')
puts SemanticPuppet::Dependency.resolve(g).map { |r| [r.name, r.version.to_s] }.sort.inspect

N = 500
20.times { SemanticPuppet::Dependency.resolve(g) } # warm-up
t = Benchmark.realtime { N.times { SemanticPuppet::Dependency.resolve(g) } }
puts "resolve_ns_per_op #{(t / N * 1e9).round}"

M = 300
t2 = Benchmark.realtime do
  M.times do
    SemanticPuppet::Dependency.clear_sources
    SemanticPuppet::Dependency.add_source(build_universe)
    gg = SemanticPuppet::Dependency.query('mod00' => '>= 1.0.0')
    SemanticPuppet::Dependency.resolve(gg)
  end
end
puts "query_resolve_cold_ns_per_op #{(t2 / M * 1e9).round}"
```

[`Resolve`]: ./dependency.go
[`Query`]: ./dependency.go
