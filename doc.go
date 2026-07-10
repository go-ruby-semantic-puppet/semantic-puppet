// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

// Package semanticpuppet is a pure-Go (no cgo) port of the Ruby
// semantic_puppet gem. It implements the two value types that Puppet uses to
// reason about module versions and dependency constraints:
//
//   - [Version] parses and compares strings that conform to the Semantic
//     Versioning 2.0.0 specification (https://semver.org). It exposes the
//     MAJOR.MINOR.PATCH triple, the optional pre-release and build-metadata
//     identifiers, SemVer precedence (build metadata is ignored), stability
//     queries and next-version bumps.
//
//   - [VersionRange] parses and evaluates the version-range grammar that
//     semantic_puppet accepts (which follows node-semver): plain versions,
//     the comparators <, <=, >, >=, =, hyphen ranges (1.0.0 - 2.0.0), the
//     tilde (~, ~>, ~=) and caret (^) approximation operators, x-ranges
//     (1.x, 1.2.x, *) and logical-or (||) unions. A range answers membership
//     ([VersionRange.Include]/[VersionRange.Cover]) and can be intersected
//     with another range ([VersionRange.Intersection]).
//
// On top of these value types it ports the gem's module-dependency graph
// solver (SemanticPuppet::Dependency):
//
//   - [Source] is the injectable provider seam that yields the available
//     [ModuleRelease] values for a module name; [AddSource]/[Sources]/
//     [ClearSources] manage the source list as the gem's singleton does.
//
//   - [Query] builds a dependency [Graph] for a set of top-level constraints
//     and fetches the transitive universe of releases; [Resolve] walks that
//     graph, backtracking on conflict and preferring the newest satisfying
//     (stable-over-prerelease) release, and returns the resolved release set
//     or an [*UnsatisfiableGraph]. Module- and graph-level constraints
//     ([Graph.AddConstraint]/[Graph.AddGraphConstraint]) and dependency
//     cycles are honoured exactly as in the gem.
//
// The port is faithful to the observable behaviour of the gem, including its
// prerelease-inclusion rule: a pre-release version is only matched by a range
// when some clause of the range explicitly names a pre-release sharing the
// same MAJOR.MINOR.PATCH triple.
//
// The package is pure Go, uses only the standard library and has no dependency
// on any Ruby runtime; every value type is Go-typed so that a Ruby binding
// layer (such as go-embedded-ruby) can marshal Ruby values onto these types.
package semanticpuppet
