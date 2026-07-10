// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"errors"
	"sort"
	"strings"
	"testing"
)

// mkRel is a helper that builds a release via CreateRelease, failing the test
// on error.
func mkRel(t *testing.T, src Source, name, version string, deps map[string]string) *ModuleRelease {
	t.Helper()
	r, err := CreateRelease(src, name, version, deps)
	if err != nil {
		t.Fatalf("CreateRelease(%s@%s): %v", name, version, err)
	}
	return r
}

// ---------------------------------------------------------------------------
// Source / CreateRelease
// ---------------------------------------------------------------------------

func TestBaseSourcePriority(t *testing.T) {
	if (BaseSource{}).Priority() != 0 {
		t.Fatal("BaseSource priority should default to 0")
	}
}

func TestCreateRelease(t *testing.T) {
	src := newFakeSource()

	if _, err := CreateRelease(src, "foo", "not-a-version", nil); !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("bad version: err = %v", err)
	}
	if _, err := CreateRelease(src, "foo", "1.0.0", map[string]string{"bar": "<<<"}); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("bad range: err = %v", err)
	}

	// An empty dependency range string defaults to ">= 0.0.0".
	rel := mkRel(t, src, "foo", "1.0.0", map[string]string{"bar": ""})
	if got := rel.constraints["bar"][0].desc; got != ">= 0.0.0" {
		t.Fatalf("default range desc = %q, want >= 0.0.0", got)
	}
}

// ---------------------------------------------------------------------------
// ModuleRelease behaviour (mirrors module_release_spec.rb)
// ---------------------------------------------------------------------------

func TestModuleReleaseDependencyNames(t *testing.T) {
	src := newFakeSource()
	if len(mkRel(t, src, "m", "1.2.3", nil).DependencyNames()) != 0 {
		t.Fatal("no-dep release should have no dependency names")
	}
	three := mkRel(t, src, "m", "1.2.3", map[string]string{"foo": "1.0.0", "bar": "2.0.0", "baz": "3.0.0"})
	names := append([]string(nil), three.DependencyNames()...)
	sort.Strings(names)
	if strings.Join(names, ",") != "bar,baz,foo" {
		t.Fatalf("dependency_names = %v", names)
	}
}

func TestModuleReleaseString(t *testing.T) {
	s := mkRel(t, newFakeSource(), "foobarbaz", "1.2.3", nil).String()
	if !strings.Contains(s, "foobarbaz") || !strings.Contains(s, "1.2.3") {
		t.Fatalf("to_s = %q", s)
	}
}

func TestModuleReleaseAddAndSatisfied(t *testing.T) {
	src := newFakeSource()

	one := mkRel(t, src, "module", "1.2.3", map[string]string{"foo": "1.0.0"})
	one.Add(mkRel(t, src, "foo", "1.0.0", nil))
	if !one.Satisfied() {
		t.Fatal("matching dependency should satisfy")
	}

	// Mis-matching name is not appended.
	mis := mkRel(t, src, "module", "1.2.3", map[string]string{"foo": "1.0.0"})
	mis.Add(mkRel(t, src, "WAT", "1.0.0", nil))
	if mis.Satisfied() {
		t.Fatal("mismatched name should not satisfy")
	}

	// Mis-matching version is not appended.
	misv := mkRel(t, src, "module", "1.2.3", map[string]string{"foo": "1.0.0"})
	misv.Add(mkRel(t, src, "foo", "0.0.1", nil))
	if misv.Satisfied() {
		t.Fatal("mismatched version should not satisfy")
	}

	// No dependencies is trivially satisfied.
	if !mkRel(t, src, "module", "1.2.3", nil).Satisfied() {
		t.Fatal("no-dep release should be satisfied")
	}

	// Not all dependencies satisfied.
	three := mkRel(t, src, "module", "1.2.3", map[string]string{"foo": "1.0.0", "bar": "2.0.0", "baz": "3.0.0"})
	for _, ver := range []string{"0.9.0", "1.0.0", "1.0.1"} {
		three.Add(mkRel(t, src, "foo", ver, nil))
	}
	if three.Satisfied() {
		t.Fatal("only one of three deps satisfied")
	}
}

func TestModuleReleaseCompare(t *testing.T) {
	src := newFakeSource()
	// Greater/lesser version.
	if mkRel(t, src, "foo", "1.0.0", nil).Compare(mkRel(t, src, "foo", "0.1.0", nil)) <= 0 {
		t.Fatal("1.0.0 should be greater than 0.1.0")
	}
	// Ordered by name first.
	if mkRel(t, src, "bar", "2.0.0", nil).Compare(mkRel(t, src, "foo", "1.0.0", nil)) >= 0 {
		t.Fatal("bar should sort before foo regardless of version")
	}
	// Ordered by source priority first.
	lo := newFakeSource()
	lo.prio = 0
	hi := newFakeSource()
	hi.prio = 5
	if mkRel(t, lo, "foo", "9.0.0", nil).Compare(mkRel(t, hi, "foo", "1.0.0", nil)) >= 0 {
		t.Fatal("lower-priority source should sort first")
	}
}

func TestModuleReleaseEql(t *testing.T) {
	src := newFakeSource()
	a := mkRel(t, src, "foo", "1.0.0", nil)

	if !a.Eql(a) {
		t.Fatal("identity should be equal")
	}
	if a.Eql(nil) {
		t.Fatal("nil should never be equal")
	}
	if !a.Eql(mkRel(t, src, "foo", "1.0.0", nil)) {
		t.Fatal("same name+version+deps should be equal")
	}
	if a.Eql(mkRel(t, src, "bar", "1.0.0", nil)) {
		t.Fatal("different name should differ")
	}
	if a.Eql(mkRel(t, src, "foo", "1.0.1", nil)) {
		t.Fatal("different version should differ")
	}
	// Different number of dependencies.
	if a.Eql(mkRel(t, src, "foo", "1.0.0", map[string]string{"x": "1.0.0"})) {
		t.Fatal("differing dep count should differ")
	}
	// Same dep count, different dep name (key missing).
	d1 := mkRel(t, src, "foo", "1.0.0", map[string]string{"x": "1.0.0"})
	d2 := mkRel(t, src, "foo", "1.0.0", map[string]string{"y": "1.0.0"})
	if d1.Eql(d2) {
		t.Fatal("differing dep name should differ")
	}
	// Same dep name, different populated candidate list length.
	e1 := mkRel(t, src, "foo", "1.0.0", map[string]string{"x": ">= 0.0.0"})
	e2 := mkRel(t, src, "foo", "1.0.0", map[string]string{"x": ">= 0.0.0"})
	e1.Add(mkRel(t, src, "x", "1.0.0", nil))
	if e1.Eql(e2) {
		t.Fatal("differing candidate-list length should differ")
	}
	// Same dep name, same length, different candidate element.
	f1 := mkRel(t, src, "foo", "1.0.0", map[string]string{"x": ">= 0.0.0"})
	f2 := mkRel(t, src, "foo", "1.0.0", map[string]string{"x": ">= 0.0.0"})
	f1.Add(mkRel(t, src, "x", "1.0.0", nil))
	f2.Add(mkRel(t, src, "x", "2.0.0", nil))
	if f1.Eql(f2) {
		t.Fatal("differing candidate element should differ")
	}
	// Same dep name, same length, equal-by-value candidate element.
	g1 := mkRel(t, src, "foo", "1.0.0", map[string]string{"x": ">= 0.0.0"})
	g2 := mkRel(t, src, "foo", "1.0.0", map[string]string{"x": ">= 0.0.0"})
	g1.Add(mkRel(t, src, "x", "1.0.0", nil))
	g2.Add(mkRel(t, src, "x", "1.0.0", nil))
	if !g1.Eql(g2) {
		t.Fatal("equal-by-value candidate element should be equal")
	}
}

func TestSatisfiesDependency(t *testing.T) {
	src := newFakeSource()
	one := mkRel(t, src, "module", "1.2.3", map[string]string{"foo": "1.0.0"})

	// Unknown dependency name.
	if mkRel(t, src, "module", "1.2.3", nil).SatisfiesDependency(mkRel(t, src, "foo", "1.0.0", nil)) {
		t.Fatal("release with no deps satisfies nothing")
	}
	// Wrong name.
	if one.SatisfiesDependency(mkRel(t, src, "bar", "1.0.0", nil)) {
		t.Fatal("wrong name should not satisfy")
	}
	// Wrong version.
	if one.SatisfiesDependency(mkRel(t, src, "foo", "4.0.0", nil)) {
		t.Fatal("wrong version should not satisfy")
	}
	// Match.
	if !one.SatisfiesDependency(mkRel(t, src, "foo", "1.0.0", nil)) {
		t.Fatal("matching release should satisfy")
	}
}

func TestAddUnknownNameSkipped(t *testing.T) {
	src := newFakeSource()
	one := mkRel(t, src, "module", "1.2.3", map[string]string{"foo": "1.0.0"})
	one.Add(mkRel(t, src, "baz", "1.0.0", nil)) // baz is not a dependency
	if len(one.Dependencies()["baz"]) != 0 {
		t.Fatal("unknown dependency name must not be recorded")
	}
}

// ---------------------------------------------------------------------------
// PopulateChildren / Children (mirrors graph_node_spec.rb)
// ---------------------------------------------------------------------------

func TestPopulateChildren(t *testing.T) {
	// The graph depends on both foo and bar; foo also depends on bar. Mirroring
	// GraphNode#populate_children, each node is populated from the parent's
	// satisfying subset, and the recursion terminates on the shared bar node.
	src := newFakeSource()
	src.add(t, "foo", []string{"1.0.0"}, map[string]string{"bar": "1.x"})
	src.add(t, "bar", []string{"1.0.0"}, nil)
	ClearSources()
	AddSource(src)
	g, err := Query(map[string]string{"foo": "1.x", "bar": "1.x"})
	if err != nil {
		t.Fatal(err)
	}
	sol, err := Resolve(g)
	if err != nil {
		t.Fatal(err)
	}
	g.PopulateChildren(sol)
	if g.Children()["foo"] == nil || g.Children()["bar"] == nil {
		t.Fatalf("graph children incomplete: %v", g.Children())
	}
	// foo, populated from the graph's satisfying subset, records bar as a child.
	foo := g.Children()["foo"]
	if foo.Children()["bar"] == nil {
		t.Fatalf("foo child bar missing: %v", foo.Children())
	}
	// Calling again is a no-op (children already populated).
	before := len(g.Children())
	g.PopulateChildren(sol)
	if len(g.Children()) != before {
		t.Fatal("PopulateChildren should be idempotent")
	}
}

// ---------------------------------------------------------------------------
// Graph accessors
// ---------------------------------------------------------------------------

func TestGraphModules(t *testing.T) {
	ClearSources()
	AddSource(newFakeSource())
	g, err := Query(map[string]string{"foo": "1.0.0", "bar": "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	mods := g.Modules()
	sort.Strings(mods)
	if strings.Join(mods, ",") != "bar,foo" {
		t.Fatalf("modules = %v", mods)
	}
}

// ---------------------------------------------------------------------------
// UnsatisfiableGraph message assembly
// ---------------------------------------------------------------------------

func TestSentenceFromList(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b, and c"},
	}
	for _, c := range cases {
		if got := sentenceFromList(c.in); got != c.want {
			t.Fatalf("sentenceFromList(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUnsatisfiableGraphNoUnsatisfied(t *testing.T) {
	e := newUnsatisfiableGraph([]string{"a", "b"}, "")
	if e.Unsatisfied != "" {
		t.Fatal("Unsatisfied should be empty")
	}
	if !strings.Contains(e.Error(), "Could not find satisfying releases for a and b") {
		t.Fatalf("message = %q", e.Error())
	}
	// Sanity: it satisfies the error interface.
	var err error = e
	if err.Error() == "" {
		t.Fatal("empty error string")
	}
}

func TestUnsatisfiableEmptyBookkeeping(t *testing.T) {
	r := &resolver{moduleDependencies: []string{"a", "b"}, satisfieds: []string{"a", "b"}}
	if got := r.unsatisfiable(); got != "" {
		t.Fatalf("unsatisfiable() = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// Low-level walk / intersect coverage
// ---------------------------------------------------------------------------

// TestWalkConsideringConstraintConflict exercises the gem's rule that a chosen
// release's own constraints reject a conflicting transitive pick. NewModuleRelease
// pairs every constraint with a dependency (so merge pre-filters candidates),
// so this defensive branch is driven directly against walk.
func TestWalkConsideringConstraintConflict(t *testing.T) {
	src := newFakeSource()
	// "a" constrains "c" to >= 2.0.0 (and depends on it).
	a := mkRel(t, src, "a", "1.0.0", map[string]string{"c": ">= 2.0.0"})
	// Candidate c@1.0.0 violates a's constraint.
	c1 := mkRel(t, src, "c", "1.0.0", nil)

	g := newGraph(map[string]*VersionRange{"a": MustParseRange(">= 0.0.0")})
	r := &resolver{}
	deps := map[string][]*ModuleRelease{"c": {c1}}
	if sol, ok := r.walk(g, deps, []*ModuleRelease{a}); ok {
		t.Fatalf("walk should fail when a considering constraint is violated, got %v", sol)
	}
}

func TestIntersectReleasesDedup(t *testing.T) {
	src := newFakeSource()
	x := mkRel(t, src, "x", "1.0.0", nil)
	// a contains x twice; b contains x once. The result must contain x once.
	got := intersectReleases([]*ModuleRelease{x, x}, []*ModuleRelease{x})
	if len(got) != 1 || got[0] != x {
		t.Fatalf("intersectReleases dedup = %v", got)
	}
}
