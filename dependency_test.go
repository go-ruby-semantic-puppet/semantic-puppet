// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test fixtures: a fixed-universe Source (the injectable seam).
// ---------------------------------------------------------------------------

// fakeSource is a Source backed by an in-memory map, mirroring the doubles the
// gem's specs use. It is the "fixed universe of releases" the resolver walks.
type fakeSource struct {
	BaseSource
	prio     int
	releases map[string][]*ModuleRelease
	fetched  map[string]int // records how many times each name was fetched
}

func newFakeSource() *fakeSource {
	return &fakeSource{releases: map[string][]*ModuleRelease{}, fetched: map[string]int{}}
}

func (s *fakeSource) Priority() int { return s.prio }

func (s *fakeSource) Fetch(name string) []*ModuleRelease {
	s.fetched[name]++
	return s.releases[name]
}

// add registers releases of name at each version, all sharing deps.
func (s *fakeSource) add(t *testing.T, name string, versions []string, deps map[string]string) {
	t.Helper()
	for _, ver := range versions {
		rel, err := CreateRelease(s, name, ver, deps)
		if err != nil {
			t.Fatalf("CreateRelease(%s, %s): %v", name, ver, err)
		}
		s.releases[name] = append(s.releases[name], rel)
	}
}

// resolveSpecs queries and resolves the given specs against src, returning
// [name, version] pairs of the solution.
func resolveSpecs(t *testing.T, src Source, specs map[string]string, tweak func(*Graph)) ([][2]string, error) {
	t.Helper()
	ClearSources()
	AddSource(src)
	graph, err := Query(specs)
	if err != nil {
		t.Fatalf("Query(%v): %v", specs, err)
	}
	if tweak != nil {
		tweak(graph)
	}
	if len(graph.Dependencies()) == 0 {
		t.Fatalf("graph has no dependencies")
	}
	sol, err := Resolve(graph)
	if err != nil {
		return nil, err
	}
	out := make([][2]string, len(sol))
	for i, rel := range sol {
		out[i] = [2]string{rel.Name(), rel.Version().String()}
	}
	return out, nil
}

func solutionHas(sol [][2]string, name, version string) bool {
	for _, p := range sol {
		if p[0] == name && p[1] == version {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// .sources / .add_source / .clear_sources
// ---------------------------------------------------------------------------

func TestSources(t *testing.T) {
	ClearSources()
	if len(Sources()) != 0 {
		t.Fatal("sources should default to empty")
	}
	AddSource(newFakeSource())
	if len(Sources()) != 1 {
		t.Fatal("add_source should append")
	}
	// The returned slice is a copy; mutating it must not affect internal state.
	got := Sources()
	got[0] = nil
	if Sources()[0] == nil {
		t.Fatal("Sources() must return a copy")
	}
	ClearSources()
	if len(Sources()) != 0 {
		t.Fatal("clear_sources should empty the list")
	}
}

// ---------------------------------------------------------------------------
// .query
// ---------------------------------------------------------------------------

func TestQueryWithoutSources(t *testing.T) {
	ClearSources()
	g, err := Query(map[string]string{"module_name": "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if g.Satisfied() {
		t.Fatal("a query with no available releases must be unsatisfied")
	}
}

func TestQueryFetchesEachDependencyOnce(t *testing.T) {
	src := newFakeSource()
	src.add(t, "module_name", []string{"1.0.0"}, map[string]string{"bar": "1.0.0", "baz": "0.0.2"})
	src.add(t, "bar", []string{"1.0.0"}, map[string]string{"baz": "0.0.3"})
	src.add(t, "baz", []string{"0.0.2"}, nil)
	ClearSources()
	AddSource(src)
	if _, err := Query(map[string]string{"module_name": "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if src.fetched["baz"] != 1 {
		t.Fatalf("baz fetched %d times, want 1", src.fetched["baz"])
	}
	if src.fetched["module_name"] != 1 || src.fetched["bar"] != 1 {
		t.Fatalf("unexpected fetch counts: %v", src.fetched)
	}
}

func TestQueryPopulatesRelatedDependencies(t *testing.T) {
	src := newFakeSource()
	src.add(t, "foo", []string{"1.0.0"}, map[string]string{"bar": "1.0.0"})
	src.add(t, "bar", []string{"1.0.0"}, map[string]string{"baz": "0.1.0"})
	src.add(t, "baz", []string{"0.1.0"}, map[string]string{"baz": "1.0.0"})
	ClearSources()
	AddSource(src)
	g, err := Query(map[string]string{"foo": "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	foo := g.Dependencies()["foo"]
	if len(foo) != 1 || foo[0].Name() != "foo" {
		t.Fatalf("graph.dependencies[foo] = %v", foo)
	}
	bar := foo[0].Dependencies()["bar"]
	if len(bar) != 1 || bar[0].Name() != "bar" {
		t.Fatalf("foo.dependencies[bar] = %v", bar)
	}
	baz := bar[0].Dependencies()["baz"]
	if len(baz) != 1 || baz[0].Version().String() != "0.1.0" {
		t.Fatalf("bar.dependencies[baz] = %v", baz)
	}
}

func TestQueryDependencyNames(t *testing.T) {
	src := newFakeSource()
	ClearSources()
	AddSource(src)
	g, err := Query(map[string]string{"foo": "1.0.0", "bar": "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	names := append([]string(nil), g.DependencyNames()...)
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"bar", "foo"}) {
		t.Fatalf("dependency_names = %v", names)
	}
}

func TestQueryInvalidRange(t *testing.T) {
	ClearSources()
	if _, err := Query(map[string]string{"foo": "<<<"}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("Query with bad range: err = %v", err)
	}
}

func TestMultipleSourcesDedup(t *testing.T) {
	s1 := newFakeSource()
	s1.add(t, "foo", []string{"1.0.0"}, nil)
	s2 := newFakeSource()
	s2.add(t, "foo", []string{"1.0.0", "2.0.0"}, nil) // 1.0.0 is a duplicate
	ClearSources()
	AddSource(s1)
	AddSource(s2)
	got := FetchReleases("foo")
	if len(got) != 2 {
		t.Fatalf("fetch_releases deduped wrong: got %d releases, want 2", len(got))
	}
	// First source wins for the shared version.
	if got[0].source != s1 {
		t.Fatal("first source should win for duplicate versions")
	}
}

// ---------------------------------------------------------------------------
// .resolve — table-driven, mirroring dependency_spec.rb
// ---------------------------------------------------------------------------

func TestResolve(t *testing.T) {
	type mod struct {
		name     string
		versions []string
		deps     map[string]string
	}
	tests := []struct {
		name    string
		specs   map[string]string
		mods    []mod
		tweak   func(*Graph)
		want    [][2]string // required members of the solution
		notWant [][2]string // versions that must NOT appear
		errRe   []string    // substrings the error must contain (failure case)
	}{
		{
			name:  "no deps: greatest release matching range",
			specs: map[string]string{"foo": "1.x"},
			mods:  []mod{{"foo", []string{"0.9.0", "1.0.0", "1.1.0", "2.0.0"}, nil}},
			want:  [][2]string{{"foo", "1.1.0"}},
		},
		{
			name:  "no deps: greatest stable matching range over a prerelease",
			specs: map[string]string{"foo": "1.x"},
			mods:  []mod{{"foo", []string{"0.9.0", "1.0.0", "1.1.0", "1.2.0-pre", "2.0.0"}, nil}},
			want:  [][2]string{{"foo", "1.1.0"}},
		},
		{
			name:  "no deps: greatest prerelease when no stable in range (A)",
			specs: map[string]string{"foo": ">1.1.0-a <2.0.0"},
			mods:  []mod{{"foo", []string{"1.0.0", "1.1.0-a", "1.1.0-b", "2.0.0"}, nil}},
			want:  [][2]string{{"foo", "1.1.0-b"}},
		},
		{
			name:  "no deps: greatest prerelease when no stable in range (B)",
			specs: map[string]string{"foo": "1.1.0-a"},
			mods:  []mod{{"foo", []string{"1.0.0", "1.1.0-a", "1.1.0-b", "2.0.0"}, nil}},
			want:  [][2]string{{"foo", "1.1.0-a"}},
		},
		{
			name:  "no deps: no version in range fails",
			specs: map[string]string{"foo": "2.x"},
			mods:  []mod{{"foo", []string{"1.0.0", "1.1.0-a", "1.1.0"}, nil}},
			errRe: []string{"Could not find satisfying releases", "foo"},
		},
		{
			name:  "with deps: greatest release matching dependency range",
			specs: map[string]string{"foo": "1.1.0"},
			mods: []mod{
				{"foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"}},
				{"bar", []string{"0.9.0", "1.0.0", "1.1.0", "1.2.0", "2.0.0"}, nil},
			},
			want: [][2]string{{"foo", "1.1.0"}, {"bar", "1.2.0"}},
		},
		{
			name:  "with deps: greatest stable dependency over prerelease",
			specs: map[string]string{"foo": "1.1.0"},
			mods: []mod{
				{"foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"}},
				{"bar", []string{"0.9.0", "1.0.0", "1.1.0", "1.2.0-pre", "2.0.0"}, nil},
			},
			want: [][2]string{{"foo", "1.1.0"}, {"bar", "1.1.0"}},
		},
		{
			name:  "with deps: unsatisfiable dependency fails",
			specs: map[string]string{"foo": "1.1.0"},
			mods: []mod{
				{"foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"}},
				{"bar", []string{"0.0.1", "0.1.0-a", "0.1.0"}, nil},
			},
			errRe: []string{"Could not find satisfying releases", "foo"},
		},
		{
			name:  "competing deps that overlap: greatest satisfying all",
			specs: map[string]string{"foo": "1.1.0"},
			mods: []mod{
				{"foo", []string{"1.1.0"}, map[string]string{"bar": "1.0.0", "baz": "1.0.0"}},
				{"bar", []string{"1.0.0"}, map[string]string{"quxx": "1.x"}},
				{"baz", []string{"1.0.0"}, map[string]string{"quxx": "1.1.x"}},
				{"quxx", []string{"0.9.0", "1.0.0", "1.1.0", "1.1.1", "1.2.0", "2.0.0"}, nil},
			},
			want:    [][2]string{{"quxx", "1.1.1"}},
			notWant: [][2]string{{"quxx", "1.2.0"}},
		},
		{
			name:  "competing deps that do not overlap: fails",
			specs: map[string]string{"foo": "1.1.0"},
			mods: []mod{
				{"foo", []string{"1.1.0"}, map[string]string{"bar": "1.0.0", "baz": "1.0.0"}},
				{"bar", []string{"1.0.0"}, map[string]string{"quxx": "1.x"}},
				{"baz", []string{"1.0.0"}, map[string]string{"quxx": "2.x"}},
				{"quxx", []string{"0.9.0", "1.0.0", "1.1.0", "1.1.1", "1.2.0", "2.0.0"}, nil},
			},
			errRe: []string{"Could not find satisfying releases", "foo"},
		},
		{
			name:  "circular deps that resolve: terminates",
			specs: map[string]string{"foo": "1.1.0"},
			mods:  []mod{{"foo", []string{"1.1.0"}, map[string]string{"foo": "1.x"}}},
			want:  [][2]string{{"foo", "1.1.0"}},
		},
		{
			name:  "circular deps that cannot resolve: fails",
			specs: map[string]string{"foo": "1.1.0"},
			mods:  []mod{{"foo", []string{"1.1.0"}, map[string]string{"foo": "1.0.0"}}},
			errRe: []string{"Could not find satisfying releases", "foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newFakeSource()
			for _, m := range tt.mods {
				src.add(t, m.name, m.versions, m.deps)
			}
			sol, err := resolveSpecs(t, src, tt.specs, tt.tweak)
			if len(tt.errRe) > 0 {
				if err == nil {
					t.Fatalf("expected failure, got solution %v", sol)
				}
				for _, re := range tt.errRe {
					if !strings.Contains(err.Error(), re) {
						t.Fatalf("error %q missing %q", err.Error(), re)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected failure: %v", err)
			}
			for _, w := range tt.want {
				if !solutionHas(sol, w[0], w[1]) {
					t.Fatalf("solution %v missing %v", sol, w)
				}
			}
			for _, nw := range tt.notWant {
				if solutionHas(sol, nw[0], nw[1]) {
					t.Fatalf("solution %v must not contain %v", sol, nw)
				}
			}
		})
	}
}

// TestResolveNoStableDependency mirrors the "dependency has no stable versions"
// spec: the constraint is imposed by the specific foo release chosen.
func TestResolveNoStableDependency(t *testing.T) {
	build := func() *fakeSource {
		src := newFakeSource()
		src.add(t, "foo", []string{"1.1.0"}, map[string]string{"bar": ">=1.1.0-0 <1.2.0"})
		src.add(t, "foo", []string{"1.1.1"}, map[string]string{"bar": "1.1.0-a"})
		src.add(t, "bar", []string{"1.0.0", "1.1.0-a", "1.1.0-b", "2.0.0"}, nil)
		return src
	}
	sol, err := resolveSpecs(t, build(), map[string]string{"foo": "1.1.0"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !solutionHas(sol, "foo", "1.1.0") || !solutionHas(sol, "bar", "1.1.0-b") {
		t.Fatalf("solution = %v, want foo 1.1.0 + bar 1.1.0-b", sol)
	}
	sol, err = resolveSpecs(t, build(), map[string]string{"foo": "1.1.1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !solutionHas(sol, "foo", "1.1.1") || !solutionHas(sol, "bar", "1.1.0-a") {
		t.Fatalf("solution = %v, want foo 1.1.1 + bar 1.1.0-a", sol)
	}
}

// TestResolveSetsUnsatisfiable mirrors the "sets unsatisfiable" spec.
func TestResolveSetsUnsatisfiable(t *testing.T) {
	src := newFakeSource()
	src.add(t, "foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"})
	src.add(t, "bar", []string{"0.0.1", "0.1.0-a", "0.1.0"}, nil)
	_, err := resolveSpecs(t, src, map[string]string{"foo": "1.1.0"}, nil)
	if err == nil {
		t.Fatal("expected failure")
	}
	if Unsatisfiable() != "foo" {
		t.Fatalf("Unsatisfiable() = %q, want foo", Unsatisfiable())
	}
}

// TestResolveModuleGraphConstraint mirrors the "violate module constraints on
// the graph" specs (a graph-level AddConstraint on a named module).
func TestResolveModuleGraphConstraint(t *testing.T) {
	noDowngrade := func(g *Graph) {
		rng := MustParseRange("> 3.0.0")
		g.AddConstraint("no downgrade", "bar", "> 3.0.0", func(node *ModuleRelease) bool {
			return rng.Include(node.Version())
		})
	}

	t.Run("resolvable", func(t *testing.T) {
		src := newFakeSource()
		src.add(t, "foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"})
		src.add(t, "foo", []string{"1.2.0"}, map[string]string{"bar": ">= 2.0.0"})
		src.add(t, "bar", []string{"1.0.0"}, nil)
		src.add(t, "bar", []string{"2.0.0"}, map[string]string{"baz": ">= 1.0.0"})
		src.add(t, "bar", []string{"3.0.0"}, nil)
		src.add(t, "bar", []string{"3.0.1"}, nil)
		src.add(t, "baz", []string{"1.0.0"}, nil)
		sol, err := resolveSpecs(t, src, map[string]string{"foo": "1.x"}, noDowngrade)
		if err != nil {
			t.Fatal(err)
		}
		if !solutionHas(sol, "foo", "1.2.0") || !solutionHas(sol, "bar", "3.0.1") {
			t.Fatalf("solution = %v, want foo 1.2.0 + bar 3.0.1", sol)
		}
	})

	t.Run("unresolvable", func(t *testing.T) {
		src := newFakeSource()
		src.add(t, "foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"})
		src.add(t, "foo", []string{"1.2.0"}, map[string]string{"bar": "2.x"})
		src.add(t, "bar", []string{"1.0.0"}, map[string]string{"baz": "1.x"})
		src.add(t, "bar", []string{"2.0.0"}, map[string]string{"baz": "1.x"})
		src.add(t, "baz", []string{"1.0.0"}, nil)
		src.add(t, "baz", []string{"3.0.0"}, nil)
		src.add(t, "baz", []string{"3.0.1"}, nil)
		_, err := resolveSpecs(t, src, map[string]string{"foo": "1.x"}, noDowngrade)
		if err == nil {
			t.Fatal("expected failure")
		}
		if !strings.Contains(err.Error(), "foo") {
			t.Fatalf("error %q missing foo", err.Error())
		}
	})
}

// TestResolveGraphConstraint mirrors the "violate graph constraints" specs (a
// whole-solution AddGraphConstraint).
func TestResolveGraphConstraint(t *testing.T) {
	uniqueness := func(g *Graph) {
		g.AddGraphConstraint("uniqueness", func(nodes []*ModuleRelease) bool {
			for _, n := range nodes {
				if strings.Contains(n.Name(), "z") {
					return false
				}
			}
			return true
		})
	}

	t.Run("resolvable", func(t *testing.T) {
		src := newFakeSource()
		src.add(t, "foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"})
		src.add(t, "foo", []string{"1.2.0"}, map[string]string{"bar": "2.x"})
		src.add(t, "bar", []string{"1.0.0"}, nil)
		src.add(t, "bar", []string{"2.0.0"}, map[string]string{"baz": "1.0.0"})
		src.add(t, "baz", []string{"1.0.0"}, nil)
		sol, err := resolveSpecs(t, src, map[string]string{"foo": "1.x"}, uniqueness)
		if err != nil {
			t.Fatal(err)
		}
		if !solutionHas(sol, "foo", "1.1.0") || !solutionHas(sol, "bar", "1.0.0") {
			t.Fatalf("solution = %v, want foo 1.1.0 + bar 1.0.0", sol)
		}
	})

	t.Run("unresolvable", func(t *testing.T) {
		src := newFakeSource()
		src.add(t, "foo", []string{"1.1.0"}, map[string]string{"bar": "1.x"})
		src.add(t, "foo", []string{"1.2.0"}, map[string]string{"bar": "2.x"})
		src.add(t, "bar", []string{"1.0.0"}, map[string]string{"baz": "1.0.0"})
		src.add(t, "bar", []string{"2.0.0"}, map[string]string{"baz": "1.0.0"})
		src.add(t, "baz", []string{"1.0.0"}, nil)
		_, err := resolveSpecs(t, src, map[string]string{"foo": "1.1.0"}, uniqueness)
		if err == nil {
			t.Fatal("expected failure")
		}
		if !strings.Contains(err.Error(), "foo") {
			t.Fatalf("error %q missing foo", err.Error())
		}
	})
}
