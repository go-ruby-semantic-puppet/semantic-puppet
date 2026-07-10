// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"fmt"
	"sort"
	"strings"
)

// This file ports the SemanticPuppet::Dependency module of the Ruby
// semantic_puppet gem: the module-dependency graph solver. It reproduces the
// gem's data model (Source / ModuleRelease / Graph / GraphNode) and its
// backtracking resolver ([Resolve]) faithfully, including its preference for
// the newest satisfying (and stable-over-prerelease) release, its handling of
// cycles and missing modules, and its module- and graph-level constraints.
//
// The gem keeps its Source list and last-resolution bookkeeping as singleton
// state on the SemanticPuppet::Dependency module; the package-level functions
// ([Sources], [AddSource], [ClearSources], [Query], [Resolve], [FetchReleases]
// and [Unsatisfiable]) mirror that singleton exactly.

// ---------------------------------------------------------------------------
// Source.
// ---------------------------------------------------------------------------

// Source is the provider seam of the resolver. Given a module name it yields
// every release it knows about for that name. Implementations are injected via
// [AddSource]; the resolver never performs any I/O of its own, so tests can
// supply a fixed universe of releases.
//
// It mirrors SemanticPuppet::Dependency::Source. The base gem class also has a
// default priority of 0 and a create_release helper; [BaseSource] provides the
// former and [CreateRelease] the latter.
type Source interface {
	// Priority orders sources when the same version is offered by more than
	// one of them; a release's priority participates in release ordering.
	Priority() int
	// Fetch returns the releases the source knows about for name (possibly
	// empty). It must not return nil-typed elements.
	Fetch(name string) []*ModuleRelease
}

// BaseSource can be embedded in a Source implementation to inherit the gem's
// default priority of 0.
type BaseSource struct{}

// Priority returns 0, the default source priority.
func (BaseSource) Priority() int { return 0 }

// CreateRelease builds a [ModuleRelease] belonging to src, mirroring
// Source#create_release. version is a Semantic Version string; deps maps a
// dependency name to a version-range string (an empty string is treated as
// ">= 0.0.0", matching the gem's nil handling). It returns an error wrapping
// [ErrInvalidVersion] or [ErrInvalidRange] for malformed input.
func CreateRelease(src Source, name, version string, deps map[string]string) (*ModuleRelease, error) {
	v, err := Parse(version)
	if err != nil {
		return nil, err
	}
	ranges := make(map[string]*VersionRange, len(deps))
	for k, val := range deps {
		s := val
		if s == "" {
			s = ">= 0.0.0"
		}
		rng, err := ParseRange(s)
		if err != nil {
			return nil, err
		}
		ranges[k] = rng
	}
	return NewModuleRelease(src, name, v, ranges), nil
}

// ---------------------------------------------------------------------------
// GraphNode (shared base of Graph and ModuleRelease).
// ---------------------------------------------------------------------------

// constraint is a single named predicate on a candidate release, mirroring the
// [source, description, block] triples stored by GraphNode#add_constraint.
type constraint struct {
	source string
	desc   string
	test   func(*ModuleRelease) bool
}

// graphNode is the shared behaviour that the gem mixes in via the GraphNode
// module. Both [Graph] and [ModuleRelease] embed it.
type graphNode struct {
	deps        map[string][]*ModuleRelease
	depOrder    []string
	constraints map[string][]constraint
	children    map[string]*ModuleRelease
}

func newGraphNode() graphNode {
	return graphNode{
		deps:        map[string][]*ModuleRelease{},
		constraints: map[string][]constraint{},
	}
}

// addDependency registers name as a dependency with (initially) no satisfying
// releases. It mirrors GraphNode#add_dependency.
func (g *graphNode) addDependency(name string) {
	if _, ok := g.deps[name]; !ok {
		g.deps[name] = nil
		g.depOrder = append(g.depOrder, name)
	}
}

// addConstraint appends a predicate constraining the releases acceptable for
// mod. It mirrors GraphNode#add_constraint.
func (g *graphNode) addConstraint(source, mod, desc string, test func(*ModuleRelease) bool) {
	g.constraints[mod] = append(g.constraints[mod], constraint{source, desc, test})
}

// Dependencies returns the map of dependency name to the satisfying releases
// discovered for it. It mirrors GraphNode#dependencies.
func (g *graphNode) Dependencies() map[string][]*ModuleRelease { return g.deps }

// DependencyNames returns the dependency names in registration order. It
// mirrors GraphNode#dependency_names.
func (g *graphNode) DependencyNames() []string { return g.depOrder }

// Satisfied reports whether every registered dependency has at least one
// satisfying release. It mirrors GraphNode#satisfied?.
func (g *graphNode) Satisfied() bool {
	for _, name := range g.depOrder {
		if len(g.deps[name]) == 0 {
			return false
		}
	}
	return true
}

// SatisfiesConstraints reports whether rel passes every constraint recorded for
// rel's module name. It mirrors GraphNode#satisfies_constraints?.
func (g *graphNode) SatisfiesConstraints(rel *ModuleRelease) bool {
	for _, c := range g.constraints[rel.name] {
		if !c.test(rel) {
			return false
		}
	}
	return true
}

// SatisfiesDependency reports whether rel is a satisfying release for one of
// this node's dependencies. It mirrors GraphNode#satisfies_dependency?.
func (g *graphNode) SatisfiesDependency(rel *ModuleRelease) bool {
	if _, ok := g.deps[rel.name]; !ok {
		return false
	}
	return g.SatisfiesConstraints(rel)
}

// Add records the given releases against any matching dependency, keeping each
// dependency's release list sorted. It mirrors GraphNode#<<.
func (g *graphNode) Add(nodes ...*ModuleRelease) {
	groups := map[string][]*ModuleRelease{}
	var order []string
	for _, n := range nodes {
		if _, ok := groups[n.name]; !ok {
			order = append(order, n.name)
		}
		groups[n.name] = append(groups[n.name], n)
	}
	for _, name := range order {
		if _, ok := g.deps[name]; !ok {
			continue
		}
		changed := false
		for _, n := range groups[name] {
			if g.SatisfiesDependency(n) {
				g.deps[name] = append(g.deps[name], n)
				changed = true
			}
		}
		if changed {
			sortReleases(g.deps[name])
		}
	}
}

// Children returns the resolved child nodes populated by [PopulateChildren]. It
// mirrors GraphNode#children.
func (g *graphNode) Children() map[string]*ModuleRelease {
	if g.children == nil {
		g.children = map[string]*ModuleRelease{}
	}
	return g.children
}

// PopulateChildren walks a resolved solution and records, for each satisfied
// dependency, the release that satisfies it, recursing across the solution. It
// mirrors GraphNode#populate_children.
func (g *graphNode) PopulateChildren(nodes []*ModuleRelease) {
	if len(g.Children()) != 0 {
		return
	}
	var selected []*ModuleRelease
	for _, n := range nodes {
		if g.SatisfiesDependency(n) {
			selected = append(selected, n)
		}
	}
	for _, n := range selected {
		g.children[n.name] = n
		n.PopulateChildren(selected)
	}
}

// ---------------------------------------------------------------------------
// ModuleRelease.
// ---------------------------------------------------------------------------

// ModuleRelease is one published release of a module: a name, a [Version] and a
// set of dependency constraints. It mirrors
// SemanticPuppet::Dependency::ModuleRelease.
type ModuleRelease struct {
	graphNode
	source  Source
	name    string
	version *Version
}

// NewModuleRelease constructs a release owned by src with the given name,
// version and dependency ranges. It mirrors ModuleRelease#initialize: each
// dependency becomes both a registered dependency and a constraint requiring
// the dependency's version to fall within range.
func NewModuleRelease(src Source, name string, version *Version, deps map[string]*VersionRange) *ModuleRelease {
	mr := &ModuleRelease{
		graphNode: newGraphNode(),
		source:    src,
		name:      name,
		version:   version,
	}
	for _, depName := range sortedKeys(deps) {
		rng := deps[depName]
		mr.addConstraint("initialize", depName, rng.String(), func(node *ModuleRelease) bool {
			return rng.Include(node.version)
		})
		mr.addDependency(depName)
	}
	return mr
}

// Name returns the module name.
func (m *ModuleRelease) Name() string { return m.name }

// Version returns the release's version.
func (m *ModuleRelease) Version() *Version { return m.version }

// priority is the release's source priority, participating in ordering.
func (m *ModuleRelease) priority() int { return m.source.Priority() }

// Compare orders releases by [priority, name, version], mirroring
// ModuleRelease#<=>.
func (m *ModuleRelease) Compare(o *ModuleRelease) int {
	if c := compareInt(m.priority(), o.priority()); c != 0 {
		return c
	}
	if c := strings.Compare(m.name, o.name); c != 0 {
		return c
	}
	return m.version.Compare(o.version)
}

// Eql reports whether two releases are equal by name, version and dependency
// set, mirroring ModuleRelease#eql?/#==. A nil operand is never equal.
func (m *ModuleRelease) Eql(o *ModuleRelease) bool {
	if m == o {
		return true
	}
	if o == nil {
		return false
	}
	if m.name != o.name || !m.version.Equal(o.version) {
		return false
	}
	if len(m.deps) != len(o.deps) {
		return false
	}
	for k, av := range m.deps {
		bv, ok := o.deps[k]
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			// Contained releases are shared per resolution, so an identity
			// or (name, version) match is exact; this also terminates on the
			// cyclic structures the gem guards against by recursion detection.
			if av[i] != bv[i] &&
				(av[i].name != bv[i].name || !av[i].version.Equal(bv[i].version)) {
				return false
			}
		}
	}
	return true
}

// String renders the release for diagnostics, mirroring ModuleRelease#to_s.
func (m *ModuleRelease) String() string {
	return fmt.Sprintf("#<ModuleRelease %s@%s>", m.name, m.version)
}

// ---------------------------------------------------------------------------
// Graph.
// ---------------------------------------------------------------------------

// graphConstraint is a whole-solution predicate registered by
// [Graph.AddGraphConstraint].
type graphConstraint struct {
	source string
	test   func([]*ModuleRelease) bool
}

// Graph is the root of a dependency query: the set of top-level module
// constraints plus any additional module- or graph-level constraints. It
// mirrors SemanticPuppet::Dependency::Graph. Build one with [Query].
type Graph struct {
	graphNode
	modules          []string
	graphConstraints []graphConstraint
}

// newGraph builds a graph from a set of top-level module ranges. Module names
// are processed in sorted order so the graph is deterministic regardless of Go
// map iteration order.
func newGraph(modules map[string]*VersionRange) *Graph {
	g := &Graph{graphNode: newGraphNode()}
	for _, name := range sortedKeys(modules) {
		g.modules = append(g.modules, name)
		rng := modules[name]
		g.addConstraint("initialize", name, rng.String(), func(node *ModuleRelease) bool {
			return rng.Include(node.version)
		})
		g.addDependency(name)
	}
	return g
}

// Modules returns the top-level module names of the query. It mirrors
// Graph#modules.
func (g *Graph) Modules() []string { return g.modules }

// AddConstraint adds a module-level constraint: releases of mod must satisfy
// test. desc describes the constraint and source names its origin. It mirrors
// GraphNode#add_constraint as exposed on the graph.
func (g *Graph) AddConstraint(source, mod, desc string, test func(*ModuleRelease) bool) {
	g.addConstraint(source, mod, desc, test)
}

// AddGraphConstraint adds a whole-solution constraint: every candidate solution
// (partial or complete) must satisfy test. It mirrors Graph#add_graph_constraint.
func (g *Graph) AddGraphConstraint(source string, test func([]*ModuleRelease) bool) {
	g.graphConstraints = append(g.graphConstraints, graphConstraint{source, test})
}

// SatisfiesGraph reports whether solution violates no graph-level constraint.
// It mirrors Graph#satisfies_graph?.
func (g *Graph) SatisfiesGraph(solution []*ModuleRelease) bool {
	for _, c := range g.graphConstraints {
		if !c.test(solution) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// UnsatisfiableGraph error.
// ---------------------------------------------------------------------------

// UnsatisfiableGraph is returned by [Resolve] when no consistent set of
// releases satisfies the graph. It mirrors
// SemanticPuppet::Dependency::UnsatisfiableGraph.
type UnsatisfiableGraph struct {
	// Modules is the list of top-level module names of the graph.
	Modules []string
	// Unsatisfied is the module that could not be satisfied, or "" when the
	// resolver could not attribute the failure to a single module.
	Unsatisfied string
	msg         string
}

// Error returns the human-readable failure message.
func (e *UnsatisfiableGraph) Error() string { return e.msg }

func newUnsatisfiableGraph(modules []string, unsatisfied string) *UnsatisfiableGraph {
	deps := sentenceFromList(modules)
	e := &UnsatisfiableGraph{Modules: modules, Unsatisfied: unsatisfied}
	if unsatisfied != "" {
		e.msg = fmt.Sprintf("Could not find satisfying releases of %s for %s", unsatisfied, deps)
	} else {
		e.msg = fmt.Sprintf("Could not find satisfying releases for %s", deps)
	}
	return e
}

// sentenceFromList joins names the way UnsatisfiableGraph#sentence_from_list
// does: "a"; "a and b"; "a, b, and c".
func sentenceFromList(list []string) string {
	switch len(list) {
	case 1:
		return list[0]
	case 2:
		return list[0] + " and " + list[1]
	default:
		out := make([]string, len(list))
		copy(out, list)
		out[len(out)-1] = "and " + out[len(out)-1]
		return strings.Join(out, ", ")
	}
}

// ---------------------------------------------------------------------------
// Resolver (the SemanticPuppet::Dependency singleton).
// ---------------------------------------------------------------------------

// resolver holds the Source list and the bookkeeping the gem keeps on its
// Dependency module singleton.
type resolver struct {
	sources            []Source
	moduleDependencies []string
	satisfieds         []string
}

// defaultResolver backs the package-level functions, mirroring the gem's module
// singleton.
var defaultResolver = &resolver{}

// Sources returns a copy of the current source list, mirroring
// Dependency.sources (which returns a frozen copy).
func Sources() []Source {
	out := make([]Source, len(defaultResolver.sources))
	copy(out, defaultResolver.sources)
	return out
}

// AddSource appends a source to the list, mirroring Dependency.add_source.
func AddSource(s Source) { defaultResolver.sources = append(defaultResolver.sources, s) }

// ClearSources empties the source list, mirroring Dependency.clear_sources.
func ClearSources() { defaultResolver.sources = nil }

// Unsatisfiable returns the module that the most recent [Resolve] failure could
// not satisfy, or "" if there was none. It mirrors Dependency.unsatisfiable.
func Unsatisfiable() string { return defaultResolver.unsatisfiable() }

// FetchReleases returns every distinct release available for name across all
// sources (first source to offer a given version wins). It mirrors
// Dependency.fetch_releases.
func FetchReleases(name string) []*ModuleRelease { return defaultResolver.fetchReleases(name) }

// Query builds a dependency [Graph] for the given top-level module ranges and
// eagerly fetches the transitive universe of releases from the configured
// sources. It mirrors Dependency.query. It returns an error wrapping
// [ErrInvalidRange] if any range string is malformed.
func Query(modules map[string]string) (*Graph, error) { return defaultResolver.query(modules) }

// Resolve resolves graph into a flat list of releases satisfying every
// transitive dependency and constraint, preferring the newest satisfying
// (stable-over-prerelease) release and backtracking on conflict. It mirrors
// Dependency.resolve. On failure it returns an [*UnsatisfiableGraph].
func Resolve(graph *Graph) ([]*ModuleRelease, error) { return defaultResolver.resolve(graph) }

func (r *resolver) unsatisfiable() string {
	for _, m := range r.moduleDependencies {
		if !containsString(r.satisfieds, m) {
			return m
		}
	}
	return ""
}

func (r *resolver) fetchReleases(name string) []*ModuleRelease {
	seen := map[string]bool{}
	var out []*ModuleRelease
	for _, src := range r.sources {
		for _, dep := range src.Fetch(name) {
			k := dep.version.String()
			if !seen[k] {
				seen[k] = true
				out = append(out, dep)
			}
		}
	}
	return out
}

// dependencyNode is the shared surface fetchDependencies needs; both *Graph and
// *ModuleRelease satisfy it through the embedded graphNode.
type dependencyNode interface {
	DependencyNames() []string
	Add(...*ModuleRelease)
}

func (r *resolver) fetchDependencies(node dependencyNode, cache map[string][]*ModuleRelease) {
	for _, name := range node.DependencyNames() {
		if _, ok := cache[name]; !ok {
			cache[name] = r.fetchReleases(name)
			for _, dep := range cache[name] {
				r.fetchDependencies(dep, cache)
			}
		}
		node.Add(cache[name]...)
	}
}

func (r *resolver) query(modules map[string]string) (*Graph, error) {
	constraints := make(map[string]*VersionRange, len(modules))
	for k, v := range modules {
		rng, err := ParseRange(v)
		if err != nil {
			return nil, err
		}
		constraints[k] = rng
	}
	g := newGraph(constraints)
	r.fetchDependencies(g, map[string][]*ModuleRelease{})
	return g, nil
}

func (r *resolver) resolve(graph *Graph) ([]*ModuleRelease, error) {
	r.moduleDependencies = nil
	r.satisfieds = nil
	deps := make(map[string][]*ModuleRelease, len(graph.deps))
	for k, v := range graph.deps {
		deps[k] = v
	}
	if sol, ok := r.walk(graph, deps, nil); ok {
		return sol, nil
	}
	return nil, newUnsatisfiableGraph(graph.modules, r.unsatisfiable())
}

// walk is the backtracking core, mirroring Dependency#walk. It returns the
// accepted solution and true, or (nil, false) to signal the caller to try its
// next possibility (the gem's `throw :next`).
func (r *resolver) walk(graph *Graph, deps map[string][]*ModuleRelease, considering []*ModuleRelease) ([]*ModuleRelease, bool) {
	if len(deps) == 0 {
		return considering, true
	}

	// Select the alphabetically-first outstanding dependency.
	name := minKey(deps)
	candidates := deps[name]
	remaining := make(map[string][]*ModuleRelease, len(deps))
	for k, v := range deps {
		if k != name {
			remaining[k] = v
		}
	}

	r.moduleDependencies = appendUnique(r.moduleDependencies, name)

	// Step over a dependency already satisfied by something under consideration.
	if len(intersectReleases(candidates, considering)) > 0 {
		return r.walk(graph, remaining, considering)
	}

	// Try each acceptable release, newest first.
	pref := preferredReleases(candidates)
	for i := len(pref) - 1; i >= 0; i-- {
		dep := pref[i]

		// Skip releases violating a module-level constraint of the graph or of
		// any release already chosen.
		if !graph.SatisfiesConstraints(dep) {
			continue
		}
		conflict := false
		for _, c := range considering {
			if !c.SatisfiesConstraints(dep) {
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}

		// Skip releases violating a graph-level constraint.
		potential := make([]*ModuleRelease, len(considering)+1)
		copy(potential, considering)
		potential[len(considering)] = dep
		if !graph.SatisfiesGraph(potential) {
			continue
		}

		r.satisfieds = appendUnique(r.satisfieds, name)

		merged := mergeDeps(remaining, dep.deps)
		if sol, ok := r.walk(graph, merged, potential); ok {
			return sol, true
		}
	}

	// Exhausted every possibility for this dependency.
	return nil, false
}

// preferredReleases keeps only satisfied releases and, if any is stable, only
// the stable ones. It mirrors Dependency#preferred_releases.
func preferredReleases(candidates []*ModuleRelease) []*ModuleRelease {
	var satisfied []*ModuleRelease
	for _, c := range candidates {
		if c.Satisfied() {
			satisfied = append(satisfied, c)
		}
	}
	hasStable := false
	for _, c := range satisfied {
		if c.version.Stable() {
			hasStable = true
			break
		}
	}
	if !hasStable {
		return satisfied
	}
	var stable []*ModuleRelease
	for _, c := range satisfied {
		if c.version.Stable() {
			stable = append(stable, c)
		}
	}
	return stable
}

// mergeDeps merges add into a copy of base; keys present in both are replaced by
// the (order-preserving) intersection of their release lists. It mirrors the
// gem's `dependencies.merge(dep.dependencies) { |_,a,b| a & b }`.
func mergeDeps(base, add map[string][]*ModuleRelease) map[string][]*ModuleRelease {
	out := make(map[string][]*ModuleRelease, len(base)+len(add))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range add {
		if existing, ok := out[k]; ok {
			out[k] = intersectReleases(existing, v)
		} else {
			out[k] = v
		}
	}
	return out
}

// intersectReleases returns the releases of a that also appear in b, without
// duplicates and in a's order, mirroring Ruby's Array#&.
func intersectReleases(a, b []*ModuleRelease) []*ModuleRelease {
	var out []*ModuleRelease
	for _, x := range a {
		inB := false
		for _, y := range b {
			if x.Eql(y) {
				inB = true
				break
			}
		}
		if !inB {
			continue
		}
		dup := false
		for _, z := range out {
			if x.Eql(z) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, x)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Small helpers.
// ---------------------------------------------------------------------------

func sortReleases(rs []*ModuleRelease) {
	sort.Slice(rs, func(i, j int) bool { return rs[i].Compare(rs[j]) < 0 })
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func minKey(m map[string][]*ModuleRelease) string {
	first := true
	var min string
	for k := range m {
		if first || k < min {
			min = k
			first = false
		}
	}
	return min
}

func appendUnique(s []string, v string) []string {
	if containsString(s, v) {
		return s
	}
	return append(s, v)
}

func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
