// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"errors"
	"testing"
)

func mustRange(t *testing.T, s string) *VersionRange {
	t.Helper()
	r, err := ParseRange(s)
	if err != nil {
		t.Fatalf("ParseRange(%q) unexpected error: %v", s, err)
	}
	return r
}

// TestParseRangeInspect checks the canonical (inspect) form of each grammar,
// cross-checked against the Ruby semantic_puppet gem.
func TestParseRangeInspect(t *testing.T) {
	tests := []struct {
		in      string
		inspect string
	}{
		{"1.2.3", "1.2.3"},
		{"=1.2.3", "1.2.3"},
		{">=1.0.0 <2.0.0", ">=1.0.0 <2.0.0"},
		{"1.0.0 - 2.0.0", ">=1.0.0 <=2.0.0"},
		{"~>1.2.3", ">=1.2.3 <1.3.0"},
		{"~1.2.3", ">=1.2.3 <1.3.0"},
		{"~=1.2.3", ">=1.2.3 <1.3.0"},
		{"~1.2", ">=1.2.0 <1.3.0"},
		{"~1", ">=1.0.0 <2.0.0"},
		{"^1.2.3", ">=1.2.3 <2.0.0"},
		{"^1.2", ">=1.2.0 <2.0.0"},
		{"^1", ">=1.0.0 <2.0.0"},
		{"^0.2.3", ">=0.2.3 <0.3.0"},
		{"^0.0.3", ">=0.0.3 <0.1.0"},
		{"^0.2", ">=0.2.0 <0.3.0"},
		{"^0", ">=0.0.0 <1.0.0"},
		{"^1.2.3-alpha", ">=1.2.3-alpha <2.0.0"},
		{"1.x", ">=1.0.0 <2.0.0"},
		{"1.2.x", ">=1.2.0 <1.3.0"},
		{"1", ">=1.0.0 <2.0.0"},
		{"1.2", ">=1.2.0 <1.3.0"},
		{"*", "*"},
		{"x", "*"},
		{"", "*"},
		{">1", ">=2.0.0"},
		{">1.2", ">=1.3.0"},
		{">1.2.3", ">1.2.3"},
		{">*", "<0.0.0"},
		{">=1.2.3", ">=1.2.3"},
		{"<1.2.3", "<1.2.3"},
		{"<=1", "<2.0.0"},
		{"<=1.2", "<1.3.0"},
		{"<=1.2.3", "<=1.2.3"},
		{"<=*", "*"},
		{"^*", "*"},
		{">=1.0.0 <2.0.0 || >=3.0.0", ">=1.0.0 <2.0.0 || >=3.0.0"},
		{">= 1.0.0", ">=1.0.0"}, // whitespace after operator
		{">=v1.0.0", ">=1.0.0"}, // v-prefix directly after operator
		{">=1.2.3-alpha", ">=1.2.3-alpha"},
	}
	for _, tt := range tests {
		r := mustRange(t, tt.in)
		if got := r.Inspect(); got != tt.inspect {
			t.Errorf("ParseRange(%q).Inspect() = %q, want %q", tt.in, got, tt.inspect)
		}
	}
}

func TestParseRangeStringPreservesInput(t *testing.T) {
	r := mustRange(t, "~>1.2.3")
	if r.String() != "~>1.2.3" {
		t.Errorf("String() = %q, want ~>1.2.3", r.String())
	}
	// empty input yields the all-range whose String is "*"
	if mustRange(t, "").String() != "*" {
		t.Errorf("empty range String() = %q, want *", mustRange(t, "").String())
	}
}

func TestInclude(t *testing.T) {
	tests := []struct {
		rng  string
		ver  string
		want bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{">=1.0.0 <2.0.0", "1.0.0", true},
		{">=1.0.0 <2.0.0", "1.9.9", true},
		{">=1.0.0 <2.0.0", "2.0.0", false},
		{">=1.0.0 <2.0.0", "0.9.9", false},
		{"1.0.0 - 2.0.0", "2.0.0", true},
		{"1.0.0 - 2.0.0", "2.0.1", false},
		{"~>1.2.3", "1.2.9", true},
		{"~>1.2.3", "1.3.0", false},
		{"^1.2.3", "1.9.9", true},
		{"^1.2.3", "2.0.0", false},
		{"1.x", "1.5.0", true},
		{"1.x", "2.0.0", false},
		{"*", "123.456.789", true},
		{">1.2.3", "1.2.4", true},
		{">1.2.3", "1.2.3", false},
		{"<=1.2.3", "1.2.3", true},
		{"<=1.2.3", "1.2.4", false},
		{">=1.0.0 <2.0.0 || >=3.0.0", "3.5.0", true},
		{">=1.0.0 <2.0.0 || >=3.0.0", "2.5.0", false},
		// prerelease inclusion rules
		{">=1.0.0", "1.5.0-pre", false},
		{"*", "1.0.0-pre", true},
		{">=1.5.0-a", "1.5.0-b", true},
		{">=1.5.0-a", "1.6.0-b", false},
		{">=1.5.0-a <2.0.0", "1.5.0-b", true},
		{">=1.0.0 <2.0.0-z", "2.0.0-a", true}, // max clause names prerelease at 2.0.0
	}
	for _, tt := range tests {
		r := mustRange(t, tt.rng)
		v := MustParse(tt.ver)
		if got := r.Include(v); got != tt.want {
			t.Errorf("ParseRange(%q).Include(%q) = %v, want %v", tt.rng, tt.ver, got, tt.want)
		}
		if got := r.Cover(v); got != tt.want {
			t.Errorf("ParseRange(%q).Cover(%q) = %v, want %v", tt.rng, tt.ver, got, tt.want)
		}
	}
}

func TestMinMax(t *testing.T) {
	r := mustRange(t, ">=1.2.0 <3.0.0")
	if r.Min().String() != "1.2.0" {
		t.Errorf("Min() = %q, want 1.2.0", r.Min().String())
	}
	if r.Max().String() != "3.0.0" {
		t.Errorf("Max() = %q, want 3.0.0", r.Max().String())
	}
	// A disjoint union has no single begin/end.
	u := mustRange(t, ">=1.0.0 <2.0.0 || >=3.0.0")
	if u.Min() != nil {
		t.Errorf("Min() of union = %v, want nil", u.Min())
	}
	if u.Max() != nil {
		t.Errorf("Max() of union = %v, want nil", u.Max())
	}
}

func TestIntersection(t *testing.T) {
	tests := []struct {
		a, b    string
		inspect string
	}{
		{">=1.0.0 <3.0.0", ">=2.0.0 <4.0.0", ">=2.0.0 <3.0.0"},
		{">=1.0.0", ">=2.0.0", ">=2.0.0"},
		{"<3.0.0", "<2.0.0", "<2.0.0"},
		{">=2.0.0", "<=2.0.0", "2.0.0"}, // touch at a point
		{"*", ">=1.0.0 <2.0.0", ">=1.0.0 <2.0.0"},
		{">=3.0.0", "<2.0.0", "<0.0.0"}, // disjoint -> empty
	}
	for _, tt := range tests {
		a := mustRange(t, tt.a)
		b := mustRange(t, tt.b)
		got := a.Intersection(b)
		if got.Inspect() != tt.inspect {
			t.Errorf("(%q & %q).Inspect() = %q, want %q", tt.a, tt.b, got.Inspect(), tt.inspect)
		}
	}
	// The empty intersection matches nothing.
	empty := mustRange(t, ">=3.0.0").Intersection(mustRange(t, "<2.0.0"))
	if empty.Include(MustParse("2.5.0")) {
		t.Error("empty intersection should include nothing")
	}
}

func TestRangeEqual(t *testing.T) {
	a := mustRange(t, ">=1.0.0 <2.0.0")
	b := mustRange(t, ">=1.0.0 <2.0.0")
	c := mustRange(t, ">=1.0.0 <3.0.0")
	d := mustRange(t, ">=1.0.0 <2.0.0 || >=3.0.0")
	if !a.Equal(b) {
		t.Error("identical ranges should be Equal")
	}
	if a.Equal(c) {
		t.Error("different upper bound should not be Equal")
	}
	if a.Equal(d) {
		t.Error("different clause count should not be Equal")
	}
}

func TestMergeUnions(t *testing.T) {
	tests := []struct {
		in      string
		inspect string
	}{
		{">=1.0.0 <3.0.0 || >=2.0.0 <4.0.0", ">=1.0.0 <4.0.0"},                   // overlap
		{">=1.0.0 <2.0.0 || >=2.0.0 <3.0.0", ">=1.0.0 <3.0.0"},                   // adjacent (excl/incl)
		{"1.0.0 - 2.0.0 || 2.0.1 - 3.0.0", ">=1.0.0 <=3.0.0"},                    // adjacent via next patch
		{">=1.0.0 <2.0.0 || >=5.0.0 <6.0.0", ">=1.0.0 <2.0.0 || >=5.0.0 <6.0.0"}, // disjoint stays split
	}
	for _, tt := range tests {
		r := mustRange(t, tt.in)
		if got := r.Inspect(); got != tt.inspect {
			t.Errorf("ParseRange(%q).Inspect() = %q, want %q", tt.in, got, tt.inspect)
		}
	}
}

func TestMustParseRange(t *testing.T) {
	if MustParseRange("1.2.3").Inspect() != "1.2.3" {
		t.Fatal("MustParseRange basic failed")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("MustParseRange(invalid) did not panic")
		}
	}()
	MustParseRange("this is not a range")
}

func TestParseRangeErrors(t *testing.T) {
	invalid := []string{
		"this is not a range",
		"not",
		"> v1.0.0",  // v-prefix only stripped when adjacent to operator
		">= v1.0.0", // leaves ">=v..." then a stray "v1.0.0"
		">= x1.0.0",
		">=1.2.3-01",       // leading-zero prerelease, >=
		"<1.2.3-01",        // leading-zero prerelease, <
		">1.2.3-01",        // leading-zero prerelease, >
		"<=1.2.3-01",       // leading-zero prerelease, <=
		"~>1.2.3-01",       // leading-zero prerelease, tilde
		"^1.2.3-01",        // leading-zero prerelease, caret (minor updates)
		"^0.2.3-01",        // leading-zero prerelease, caret (patch updates)
		"=1.2.3-01",        // leading-zero prerelease, equals
		"1.2.3-01",         // leading-zero prerelease, x-range/default
		"1.2.3-01 - 2.0.0", // leading-zero prerelease, hyphen lower
		"1.0.0 - 2.0.0-01", // leading-zero prerelease, hyphen upper
	}
	for _, in := range invalid {
		r, err := ParseRange(in)
		if err == nil {
			t.Errorf("ParseRange(%q) = %v, want error", in, r)
			continue
		}
		if !errors.Is(err, ErrInvalidRange) && !errors.Is(err, ErrInvalidVersion) {
			t.Errorf("ParseRange(%q) error = %v, want a sentinel", in, err)
		}
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Kind != "range" {
			t.Errorf("ParseRange(%q) error type wrong: %v", in, err)
		}
		_ = pe.Error()
	}
}

// TestParseRangeNilElements exercises the path where a clause reduces to an
// empty (nil) matcher and is compacted away, leaving a match-nothing range.
func TestParseRangeNilElements(t *testing.T) {
	r := mustRange(t, ">=5.0.0 <1.0.0 || >=5.0.0 <1.0.0")
	if r.Inspect() != "<0.0.0" {
		t.Errorf("Inspect() = %q, want <0.0.0", r.Inspect())
	}
	if r.Include(MustParse("7.0.0")) {
		t.Error("match-nothing range should include nothing")
	}
}

// TestDigitOrZero exercises x-range bounds inside comparator/hyphen operands.
func TestDigitOrZero(t *testing.T) {
	if got := mustRange(t, "1 - 2").Inspect(); got != ">=1.0.0 <=2.0.0" {
		t.Errorf("1 - 2 => %q", got)
	}
	if got := mustRange(t, ">=1").Inspect(); got != ">=1.0.0" {
		t.Errorf(">=1 => %q", got)
	}
}
