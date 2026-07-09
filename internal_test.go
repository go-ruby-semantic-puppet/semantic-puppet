// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import "testing"

func v(s string) *Version { return MustParse(s) }

func rstr(r abstractRange) string {
	if r == nil {
		return "nil"
	}
	return r.String()
}

// mm builds a two-sided minMax matcher from a lower and an upper matcher.
func mm(lo, hi abstractRange) *minMaxRange { return newMinMax(lo, hi) }

// TestGenericIntersect drives every branch of genericIntersect directly.
func TestGenericIntersect(t *testing.T) {
	tests := []struct {
		name string
		a, b abstractRange
		want string
	}{
		{"self.lower>rng.upper", &gtEqRange{v("3.0.0")}, &ltRange{v("2.0.0")}, "nil"},
		{"self.lower==rng.upper eq", &gtEqRange{v("2.0.0")}, &ltEqRange{v("2.0.0")}, "2.0.0"},
		{"self.lower==rng.upper exclBegin", &gtRange{v("2.0.0")}, &ltEqRange{v("2.0.0")}, "nil"},
		{"self.lower==rng.upper exclEnd", &gtEqRange{v("2.0.0")}, &ltRange{v("2.0.0")}, "nil"},
		{"rng.lower>self.upper", &ltEqRange{v("2.0.0")}, &gtEqRange{v("3.0.0")}, "nil"},
		{"rng.lower==self.upper eq", &ltEqRange{v("2.0.0")}, &gtEqRange{v("2.0.0")}, "2.0.0"},
		{"rng.lower==self.upper exclEnd", &ltRange{v("2.0.0")}, &gtEqRange{v("2.0.0")}, "nil"},
		{"rng.lower==self.upper exclBegin", &ltEqRange{v("2.0.0")}, &gtRange{v("2.0.0")}, "nil"},
		{"min<,!maxUpperBound", &gtEqRange{v("1.0.0")}, &gtEqRange{v("2.0.0")}, ">=2.0.0"},
		{"maxcmp>0,min=rng,!minLowerBound", &ltRange{v("3.0.0")}, &ltRange{v("2.0.0")}, "<2.0.0"},
		{"min=self exclBegin equal", &gtRange{v("1.0.0")}, &gtEqRange{v("1.0.0")}, ">1.0.0"},
		{"max=self exclEnd equal", &ltRange{v("2.0.0")}, &ltEqRange{v("2.0.0")}, "<2.0.0"},
		{"max=rng not-excl equal", &ltEqRange{v("2.0.0")}, &ltRange{v("2.0.0")}, "<2.0.0"},
		{"newMinMax min< max<", mm(&gtEqRange{v("1.0.0")}, &ltRange{v("3.0.0")}), mm(&gtEqRange{v("2.0.0")}, &ltRange{v("4.0.0")}), ">=2.0.0 <3.0.0"},
		{"newMinMax min> max>", mm(&gtEqRange{v("2.0.0")}, &ltRange{v("4.0.0")}), mm(&gtEqRange{v("1.0.0")}, &ltRange{v("3.0.0")}), ">=2.0.0 <3.0.0"},
	}
	for _, tt := range tests {
		// Call through the interface so each type's intersect wrapper is covered.
		if got := rstr(tt.a.intersect(tt.b)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	// ltEqRange as receiver exercises its lower/upper/exclude* accessors.
	if got := rstr((&ltEqRange{v("3.0.0")}).intersect(&ltEqRange{v("2.0.0")})); got != "<=2.0.0" {
		t.Errorf("ltEqRange.intersect: got %q, want <=2.0.0", got)
	}
	// eqRange as receiver exercises its intersect wrapper and bound accessors.
	if got := rstr((&eqRange{v("1.0.0")}).intersect(&eqRange{v("2.0.0")})); got != "nil" {
		t.Errorf("eqRange.intersect disjoint: got %q, want nil", got)
	}
	if got := rstr((&eqRange{v("1.0.0")}).merge(&eqRange{v("1.0.0")})); got != "1.0.0" {
		t.Errorf("eqRange.merge self: got %q, want 1.0.0", got)
	}
}

// TestGenericMerge drives every branch of genericMerge directly.
func TestGenericMerge(t *testing.T) {
	tests := []struct {
		name string
		a, b abstractRange
		want string
	}{
		{"overlap min< max<", mm(&gtEqRange{v("1.0.0")}, &ltRange{v("3.0.0")}), mm(&gtEqRange{v("2.0.0")}, &ltRange{v("4.0.0")}), ">=1.0.0 <4.0.0"},
		{"overlap min> max>", mm(&gtEqRange{v("2.0.0")}, &ltRange{v("4.0.0")}), mm(&gtEqRange{v("1.0.0")}, &ltRange{v("3.0.0")}), ">=1.0.0 <4.0.0"},
		{"overlap exclBegin lb=gt", mm(&gtRange{v("1.0.0")}, &ltRange{v("3.0.0")}), mm(&gtEqRange{v("2.0.0")}, &ltRange{v("4.0.0")}), ">1.0.0 <4.0.0"},
		{"overlap equal incl", mm(&gtEqRange{v("1.0.0")}, &ltEqRange{v("3.0.0")}), mm(&gtEqRange{v("1.0.0")}, &ltEqRange{v("3.0.0")}), ">=1.0.0 <=3.0.0"},
		{"adjacent case1", mm(&gtEqRange{v("1.0.0")}, &ltRange{v("2.0.0")}), mm(&gtEqRange{v("2.0.0")}, &ltRange{v("3.0.0")}), ">=1.0.0 <3.0.0"},
		{"adjacent case2", mm(&gtEqRange{v("2.0.0")}, &ltRange{v("3.0.0")}), mm(&gtEqRange{v("1.0.0")}, &ltRange{v("2.0.0")}), ">=1.0.0 <3.0.0"},
		{"adjacent case3 nextpatch", mm(&gtEqRange{v("1.0.0")}, &ltEqRange{v("2.0.0")}), mm(&gtEqRange{v("2.0.1")}, &ltEqRange{v("3.0.0")}), ">=1.0.0 <=3.0.0"},
		{"adjacent case4 nextpatch", mm(&gtEqRange{v("2.0.1")}, &ltEqRange{v("3.0.0")}), mm(&gtEqRange{v("1.0.0")}, &ltEqRange{v("2.0.0")}), ">=1.0.0 <=3.0.0"},
		{"adjacent case1 exclBegin fromTo", mm(&gtRange{v("1.0.0")}, &ltRange{v("2.0.0")}), mm(&gtEqRange{v("2.0.0")}, &ltRange{v("3.0.0")}), ">1.0.0 <3.0.0"},
		{"no merge", mm(&gtEqRange{v("1.0.0")}, &ltRange{v("2.0.0")}), mm(&gtEqRange{v("5.0.0")}, &ltRange{v("6.0.0")}), "nil"},
	}
	for _, tt := range tests {
		// Call through the interface so each type's merge wrapper is covered.
		if got := rstr(tt.a.merge(tt.b)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	// Comparator matchers as merge receivers (overlapping unbounded ranges).
	comparatorMerges := []struct {
		name string
		a, b abstractRange
		want string
	}{
		{"gtEq merge gtEq", &gtEqRange{v("1.0.0")}, &gtEqRange{v("2.0.0")}, ">=1.0.0"},
		{"gt merge gt", &gtRange{v("1.0.0")}, &gtRange{v("2.0.0")}, ">1.0.0"},
		{"lt merge lt", &ltRange{v("3.0.0")}, &ltRange{v("2.0.0")}, "<3.0.0"},
		{"ltEq merge ltEq", &ltEqRange{v("3.0.0")}, &ltEqRange{v("2.0.0")}, "<=3.0.0"},
	}
	for _, tt := range comparatorMerges {
		if got := rstr(tt.a.merge(tt.b)); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

// TestComparatorTestPrereleaseMethods covers testPrerelease and upperBound on
// each comparator matcher type.
func TestComparatorTestPrereleaseMethods(t *testing.T) {
	matchers := []abstractRange{
		&gtRange{v("1.0.0-a")},
		&gtEqRange{v("1.0.0-a")},
		&ltRange{v("1.0.0-a")},
		&ltEqRange{v("1.0.0-a")},
		&eqRange{v("1.0.0-a")},
	}
	for _, m := range matchers {
		if !m.testPrerelease(v("1.0.0-b")) {
			t.Errorf("%T.testPrerelease should accept a same-tuple prerelease", m)
		}
		if m.testPrerelease(v("2.0.0-b")) {
			t.Errorf("%T.testPrerelease should reject an off-tuple prerelease", m)
		}
	}
	// upperBound on the lower-bounded matchers is always false.
	if (&gtRange{v("1.0.0")}).upperBound() || (&gtEqRange{v("1.0.0")}).upperBound() {
		t.Error("lower-bounded matchers should report upperBound false")
	}
}

func TestAllRangeMatcher(t *testing.T) {
	all := allRangeSingleton
	if all.intersect(&eqRange{v("1.0.0")}).String() != "1.0.0" {
		t.Error("allRange.intersect should return the argument")
	}
	if all.merge(&eqRange{v("1.0.0")}).String() != "*" {
		t.Error("allRange.merge should return the all-range")
	}
	if !all.eq(allRange{}) {
		t.Error("allRange should equal allRange")
	}
	if all.eq(&eqRange{v("1.0.0")}) {
		t.Error("allRange should not equal a different matcher type")
	}
	if !all.testPrerelease(v("1.0.0-a")) {
		t.Error("allRange should accept prereleases")
	}
	if all.excludeBegin() || all.excludeEnd() || all.lowerBound() || all.upperBound() {
		t.Error("allRange bound flags should all be false")
	}
	if all.lower() != versionMin || all.upper() != versionMax {
		t.Error("allRange bounds should be MIN/MAX")
	}
}

func TestCreateMinMaxNilGuard(t *testing.T) {
	// An empty set reduces to nil.
	if createMinMax() != nil {
		t.Error("createMinMax() should be nil")
	}
	// A single element passes through.
	if rstr(createMinMax(&eqRange{v("1.0.0")})) != "1.0.0" {
		t.Error("createMinMax(single) should pass through")
	}
	// A mid-reduce empty intersection short-circuits to nil.
	got := createMinMax(&gtEqRange{v("5.0.0")}, &ltRange{v("2.0.0")}, &ltRange{v("1.0.0")})
	if got != nil {
		t.Errorf("createMinMax(non-overlapping triple) = %v, want nil", got)
	}
}

func TestBoundEdgeCases(t *testing.T) {
	// lowerBound is false only when the version is the synthetic MIN.
	if (&gtEqRange{versionMin}).lowerBound() {
		t.Error("gtEqRange over MIN should not be a lower bound")
	}
	if !(&gtEqRange{v("0.0.0")}).lowerBound() {
		t.Error("gtEqRange over 0.0.0 should be a lower bound")
	}
	// upperBound is false only when the version is the synthetic MAX.
	if (&ltEqRange{versionMax}).upperBound() {
		t.Error("ltEqRange over MAX should not be an upper bound")
	}
	if !(&ltEqRange{v("9.9.9")}).upperBound() {
		t.Error("ltEqRange over 9.9.9 should be an upper bound")
	}
	// eqRange bound flags depend on MIN/MAX.
	if (&eqRange{versionMin}).lowerBound() || (&eqRange{versionMax}).upperBound() {
		t.Error("eqRange over MIN/MAX should not be bounds")
	}
	if !(&eqRange{v("1.0.0")}).lowerBound() || !(&eqRange{v("1.0.0")}).upperBound() {
		t.Error("eqRange over a real version should be a bound on both sides")
	}
	// matchNothing prints specially; a plain ltRange over MIN does not.
	if matchNothing.String() != "<0.0.0" {
		t.Errorf("matchNothing.String() = %q, want <0.0.0", matchNothing.String())
	}
	if got := (&ltRange{versionMin}).String(); got != "<0.0.0-" {
		t.Errorf("plain ltRange over MIN = %q, want <0.0.0-", got)
	}
	if emptyRange.String() != "<0.0.0" {
		t.Errorf("emptyRange.String() = %q, want <0.0.0", emptyRange.String())
	}
}

func TestMatcherEqAndInclude(t *testing.T) {
	// eq: same type + version true; type mismatch false.
	if !(&gtRange{v("1.0.0")}).eq(&gtRange{v("1.0.0")}) {
		t.Error("gtRange eq self")
	}
	if (&gtRange{v("1.0.0")}).eq(&gtEqRange{v("1.0.0")}) {
		t.Error("gtRange should not eq gtEqRange")
	}
	if (&ltRange{v("1.0.0")}).eq(&gtRange{v("1.0.0")}) {
		t.Error("ltRange should not eq gtRange")
	}
	if !(&ltEqRange{v("1.0.0")}).eq(&ltEqRange{v("1.0.0")}) {
		t.Error("ltEqRange eq self")
	}
	if !(&eqRange{v("1.0.0")}).eq(&eqRange{v("1.0.0")}) {
		t.Error("eqRange eq self")
	}
	if (&eqRange{v("1.0.0")}).eq(&eqRange{v("2.0.0")}) {
		t.Error("eqRange different version")
	}
	// minMaxRange eq: type, min and max sensitivity.
	base := mm(&gtEqRange{v("1.0.0")}, &ltRange{v("2.0.0")})
	if !base.eq(mm(&gtEqRange{v("1.0.0")}, &ltRange{v("2.0.0")})) {
		t.Error("minMax eq self")
	}
	if base.eq(&gtRange{v("1.0.0")}) {
		t.Error("minMax should not eq comparator")
	}
	if base.eq(mm(&gtEqRange{v("1.5.0")}, &ltRange{v("2.0.0")})) {
		t.Error("minMax differing min")
	}
	if base.eq(mm(&gtEqRange{v("1.0.0")}, &ltRange{v("2.5.0")})) {
		t.Error("minMax differing max")
	}
	// include for each comparator type.
	if !(&gtRange{v("1.0.0")}).include(v("1.0.1")) || (&gtRange{v("1.0.0")}).include(v("1.0.0")) {
		t.Error("gtRange include")
	}
	if !(&ltRange{v("1.0.0")}).include(v("0.9.9")) || (&ltRange{v("1.0.0")}).include(v("1.0.0")) {
		t.Error("ltRange include")
	}
	if !(&ltEqRange{v("1.0.0")}).include(v("1.0.0")) || (&ltEqRange{v("1.0.0")}).include(v("1.0.1")) {
		t.Error("ltEqRange include")
	}
	// minMaxRange.testPrerelease exercises both operands.
	pre := mm(&gtEqRange{v("1.5.0-a")}, &ltRange{v("2.0.0")})
	if !pre.testPrerelease(v("1.5.0-b")) {
		t.Error("minMax testPrerelease via min")
	}
	if pre.testPrerelease(v("3.0.0-b")) {
		t.Error("minMax testPrerelease should be false off-tuple")
	}
}

func TestRangeEqualHelper(t *testing.T) {
	if !rangeEqual(nil, nil) {
		t.Error("rangeEqual(nil,nil) should be true")
	}
	if rangeEqual(nil, &eqRange{v("1.0.0")}) {
		t.Error("rangeEqual(nil, x) should be false")
	}
	if rangeEqual(&eqRange{v("1.0.0")}, nil) {
		t.Error("rangeEqual(x, nil) should be false")
	}
	if !rangeEqual(&eqRange{v("1.0.0")}, &eqRange{v("1.0.0")}) {
		t.Error("rangeEqual(x, x) should be true")
	}
}

// TestParsePartialErrorProp covers the parsePartial error branch and each
// helper's error propagation. These operands never occur via ParseRange (the
// SIMPLE grammar validates them first), so they are exercised directly.
func TestParsePartialErrorProp(t *testing.T) {
	if _, err := parsePartial("!!"); err == nil {
		t.Error("parsePartial(!!) should error")
	}
	callers := []struct {
		name string
		fn   func(string) (abstractRange, error)
	}{
		{"parseCaret", parseCaret},
		{"parseTilde", parseTilde},
		{"parseXRange", parseXRange},
		{"parseGtVersion", parseGtVersion},
		{"parseLtEqVersion", parseLtEqVersion},
	}
	for _, c := range callers {
		if _, err := c.fn("!!"); err == nil {
			t.Errorf("%s(!!) should error", c.name)
		}
	}
	if _, err := parseVersion("!!"); err == nil {
		t.Error("parseVersion(!!) should error")
	}
}
