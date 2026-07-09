// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"errors"
	"reflect"
	"sort"
	"testing"
)

func TestParseValid(t *testing.T) {
	tests := []struct {
		in         string
		major      int
		minor      int
		patch      int
		prerelease string
		build      string
		str        string
	}{
		{"0.0.0", 0, 0, 0, "", "", "0.0.0"},
		{"1.2.3", 1, 2, 3, "", "", "1.2.3"},
		{"10.20.30", 10, 20, 30, "", "", "10.20.30"},
		{"1.2.3-alpha", 1, 2, 3, "alpha", "", "1.2.3-alpha"},
		{"1.2.3-alpha.1", 1, 2, 3, "alpha.1", "", "1.2.3-alpha.1"},
		{"1.2.3-0.3.7", 1, 2, 3, "0.3.7", "", "1.2.3-0.3.7"},
		{"1.2.3-x.7.z.92", 1, 2, 3, "x.7.z.92", "", "1.2.3-x.7.z.92"},
		{"1.2.3+build", 1, 2, 3, "", "build", "1.2.3+build"},
		{"1.2.3+build.1.2", 1, 2, 3, "", "build.1.2", "1.2.3+build.1.2"},
		{"1.2.3-a.1+b.2", 1, 2, 3, "a.1", "b.2", "1.2.3-a.1+b.2"},
		{"1.0.0--", 1, 0, 0, "-", "", "1.0.0--"},
	}
	for _, tt := range tests {
		v, err := Parse(tt.in)
		if err != nil {
			t.Fatalf("Parse(%q) unexpected error: %v", tt.in, err)
		}
		if v.Major() != tt.major || v.Minor() != tt.minor || v.Patch() != tt.patch {
			t.Errorf("Parse(%q) = %d.%d.%d, want %d.%d.%d", tt.in, v.Major(), v.Minor(), v.Patch(), tt.major, tt.minor, tt.patch)
		}
		if v.Prerelease() != tt.prerelease {
			t.Errorf("Parse(%q).Prerelease() = %q, want %q", tt.in, v.Prerelease(), tt.prerelease)
		}
		if v.Build() != tt.build {
			t.Errorf("Parse(%q).Build() = %q, want %q", tt.in, v.Build(), tt.build)
		}
		if v.String() != tt.str {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.in, v.String(), tt.str)
		}
		if !IsValid(tt.in) {
			t.Errorf("IsValid(%q) = false, want true", tt.in)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	invalid := []string{
		"",
		"1",
		"1.2",
		"1.2.3.4",
		"01.2.3",
		"1.02.3",
		"1.2.03",
		"v1.2.3",
		"1.2.3-",
		"1.2.3+",
		"1.2.3-01",       // leading-zero numeric prerelease
		"1.2.3-alpha.01", // leading-zero numeric prerelease segment
		"1.2.x",
		"a.b.c",
		"1.2.3 ",
		" 1.2.3",
		"1.2.3-beta!",
	}
	for _, in := range invalid {
		v, err := Parse(in)
		if err == nil {
			t.Errorf("Parse(%q) = %v, want error", in, v)
			continue
		}
		if !errors.Is(err, ErrInvalidVersion) {
			t.Errorf("Parse(%q) error = %v, want ErrInvalidVersion", in, err)
		}
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Kind != "version" || pe.Input != in {
			t.Errorf("Parse(%q) error type/details wrong: %v", in, err)
		}
		if IsValid(in) {
			t.Errorf("IsValid(%q) = true, want false", in)
		}
		_ = pe.Error()
	}
}

func TestMustParse(t *testing.T) {
	if MustParse("1.2.3").String() != "1.2.3" {
		t.Fatal("MustParse basic failed")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("MustParse(invalid) did not panic")
		}
	}()
	MustParse("nope")
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.2.0", "1.1.0", 1},
		{"1.1.0", "1.2.0", -1},
		{"1.0.2", "1.0.1", 1},
		{"1.0.1", "1.0.2", -1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0-alpha", 0},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha", 1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1}, // numeric < alnum
		{"1.0.0-alpha.beta", "1.0.0-alpha.1", 1},
		{"1.0.0-1", "1.0.0-2", -1},
		{"1.0.0-2", "1.0.0-1", 1},
		{"1.0.0-a", "1.0.0-b", -1},
		{"1.0.0-b", "1.0.0-a", 1},
		{"1.0.0+build", "1.0.0", 0}, // build ignored
		{"1.0.0+a", "1.0.0+b", 0},   // build ignored
	}
	for _, tt := range tests {
		a := MustParse(tt.a)
		b := MustParse(tt.b)
		if got := a.Compare(b); got != tt.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"1.2.3", "1.3.3", false},
		{"1.2.3", "2.2.3", false},
		{"1.2.3-a", "1.2.3-a", true},
		{"1.2.3-a", "1.2.3", false},     // present vs absent prerelease
		{"1.2.3-a", "1.2.3-a.b", false}, // length differs
		{"1.2.3-1", "1.2.3-1", true},
		{"1.2.3-1", "1.2.3-2", false},
		{"1.2.3-a", "1.2.3-b", false},
		{"1.2.3+x", "1.2.3+x", true},
		{"1.2.3+x", "1.2.3", false},     // present vs absent build
		{"1.2.3+x", "1.2.3+x.y", false}, // build length differs
		{"1.2.3+x", "1.2.3+y", false},   // build value differs
	}
	for _, tt := range tests {
		a := MustParse(tt.a)
		b := MustParse(tt.b)
		if got := a.Equal(b); got != tt.want {
			t.Errorf("Equal(%q,%q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
	// numeric vs string identifier at same position are not equal
	if MustParse("1.2.3-1").Equal(MustParse("1.2.3-1x")) {
		t.Error("numeric vs alnum prerelease should not be equal")
	}
}

func TestStableAndToStable(t *testing.T) {
	stable := MustParse("1.2.3")
	if !stable.Stable() {
		t.Error("1.2.3 should be stable")
	}
	if stable.ToStable() != stable {
		t.Error("ToStable on already-stable should return the same value")
	}

	pre := MustParse("1.2.3-alpha+build")
	if pre.Stable() {
		t.Error("1.2.3-alpha should not be stable")
	}
	ts := pre.ToStable()
	if ts.String() != "1.2.3+build" {
		t.Errorf("ToStable() = %q, want 1.2.3+build", ts.String())
	}
	if !ts.Stable() {
		t.Error("ToStable result should be stable")
	}
}

func TestNextBumps(t *testing.T) {
	v := MustParse("1.2.3-pre+b")
	if got := v.NextMajor().String(); got != "2.0.0" {
		t.Errorf("NextMajor = %q, want 2.0.0", got)
	}
	if got := v.NextMinor().String(); got != "1.3.0" {
		t.Errorf("NextMinor = %q, want 1.3.0", got)
	}
	if got := v.NextPatch().String(); got != "1.2.4" {
		t.Errorf("NextPatch = %q, want 1.2.4", got)
	}
}

func TestMinMaxVersion(t *testing.T) {
	if MinVersion() != nil {
		t.Error("MinVersion() with no args should be nil")
	}
	if MaxVersion() != nil {
		t.Error("MaxVersion() with no args should be nil")
	}
	vs := []*Version{
		MustParse("1.2.3"),
		MustParse("0.9.0"),
		MustParse("2.0.0-alpha"),
		MustParse("2.0.0"),
	}
	if got := MinVersion(vs...).String(); got != "0.9.0" {
		t.Errorf("MinVersion = %q, want 0.9.0", got)
	}
	if got := MaxVersion(vs...).String(); got != "2.0.0" {
		t.Errorf("MaxVersion = %q, want 2.0.0", got)
	}
}

func TestSortVersions(t *testing.T) {
	vs := []*Version{
		MustParse("2.0.0"),
		MustParse("1.0.0-alpha"),
		MustParse("1.0.0"),
		MustParse("1.0.0-beta"),
		MustParse("0.1.0"),
	}
	SortVersions(vs)
	var got []string
	for _, v := range vs {
		got = append(got, v.String())
	}
	want := []string{"0.1.0", "1.0.0-alpha", "1.0.0-beta", "1.0.0", "2.0.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SortVersions = %v, want %v", got, want)
	}
	// exercise sort.Interface directly
	if !sort.IsSorted(ByVersion(vs)) {
		t.Error("ByVersion not sorted after SortVersions")
	}
}

// TestSyntheticVersions exercises the internal MIN/MAX sentinels and the
// preID.String helper used by the range algebra.
func TestSyntheticVersions(t *testing.T) {
	if got := versionMin.String(); got != "0.0.0-" {
		t.Errorf("versionMin.String() = %q, want 0.0.0-", got)
	}
	if !versionMin.Stable() {
		t.Error("versionMin should report stable (empty prerelease)")
	}
	if got := versionMax.String(); got != "Infinity.0.0" {
		t.Errorf("versionMax.String() = %q, want Infinity.0.0", got)
	}
	// MAX compares greater than any real version and equal to itself.
	if versionMax.Compare(MustParse("999.0.0")) != 1 {
		t.Error("versionMax should exceed any real version")
	}
	if MustParse("999.0.0").Compare(versionMax) != -1 {
		t.Error("real version should be below versionMax")
	}
	if versionMax.Compare(versionMax) != 0 {
		t.Error("versionMax should equal itself")
	}
	// NextPatch on MAX preserves the infinite major.
	if !versionMax.NextPatch().inf {
		t.Error("NextPatch on versionMax should stay infinite")
	}
	// versionMin carries an (empty, present) prerelease so it sorts below 0.0.0.
	if versionMin.Compare(MustParse("0.0.0")) != -1 {
		t.Error("versionMin should sort below stable 0.0.0")
	}
	// preID string forms
	if (preID{num: 7, isNum: true}).String() != "7" {
		t.Error("numeric preID string")
	}
	if (preID{str: "beta"}).String() != "beta" {
		t.Error("alnum preID string")
	}
}
