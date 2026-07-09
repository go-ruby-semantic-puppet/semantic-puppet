// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ErrInvalidVersion is returned (wrapped) by [Parse] when a string is not a
// valid Semantic Version. Use [errors.Is] to test for it.
var ErrInvalidVersion = errors.New("invalid semantic version")

// preID is a single dot-separated pre-release identifier. A pre-release
// identifier is compared numerically when it consists solely of digits and
// lexically (ASCII) otherwise; numeric identifiers always have lower
// precedence than alphanumeric ones.
type preID struct {
	num   int
	str   string
	isNum bool
}

func (p preID) String() string {
	if p.isNum {
		return strconv.Itoa(p.num)
	}
	return p.str
}

// Version is an immutable Semantic Version 2.0.0 value. The zero value is not
// usable; construct one with [Parse].
type Version struct {
	major int
	minor int
	patch int

	// inf marks the synthetic maximum version (a major component of positive
	// infinity). It is used only internally as the open upper bound of a range.
	inf bool

	// hasPre distinguishes "no pre-release" (hasPre false) from "pre-release
	// present" (hasPre true). A present-but-empty pre-release is the synthetic
	// minimum version.
	hasPre bool
	pre    []preID

	hasBuild bool
	build    []string
}

// Semantic Version matching regex, anchored. Groups: 1=major, 2=minor,
// 3=patch, 4=prerelease, 5=build. Numeric components forbid leading zeroes.
var versionRegex = regexp.MustCompile(
	`\A(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
		`(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?` +
		`(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?\z`)

var numericPreRegex = regexp.MustCompile(`\A\d+\z`)

// versionMin is the lowest-precedence version possible: 0.0.0 with an empty
// (but present) pre-release. Because it carries a pre-release it sorts below
// the stable 0.0.0.
var versionMin = &Version{hasPre: true, pre: []preID{}}

// versionMax is the highest-precedence version possible.
var versionMax = &Version{inf: true}

// Parse parses ver as a Semantic Version 2.0.0 string of the form
// MAJOR.MINOR.PATCH, optionally followed by a "-prerelease" and/or a "+build"
// section. It returns an error wrapping [ErrInvalidVersion] for malformed
// input, including numeric pre-release identifiers with leading zeroes.
func Parse(ver string) (*Version, error) {
	m := versionRegex.FindStringSubmatch(ver)
	if m == nil {
		return nil, &ParseError{Kind: "version", Input: ver, err: ErrInvalidVersion}
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])

	pre, hasPre, err := parsePrereleaseGroup(m[4])
	if err != nil {
		return nil, &ParseError{Kind: "version", Input: ver, err: err}
	}
	build, hasBuild := parseBuildGroup(m[5])

	return &Version{
		major: major, minor: minor, patch: patch,
		hasPre: hasPre, pre: pre,
		hasBuild: hasBuild, build: build,
	}, nil
}

// MustParse is like [Parse] but panics if ver is not a valid version. It is
// intended for use with constant version strings.
func MustParse(ver string) *Version {
	v, err := Parse(ver)
	if err != nil {
		panic(err)
	}
	return v
}

// IsValid reports whether ver is a valid Semantic Version string.
func IsValid(ver string) bool {
	_, err := Parse(ver)
	return err == nil
}

// parsePrereleaseGroup parses the pre-release capture group. An empty string
// means the group was absent (no pre-release).
func parsePrereleaseGroup(s string) ([]preID, bool, error) {
	if s == "" {
		return nil, false, nil
	}
	segs := strings.Split(s, ".")
	out := make([]preID, len(segs))
	for i, seg := range segs {
		if numericPreRegex.MatchString(seg) {
			if len(seg) > 1 && seg[0] == '0' {
				return nil, false, fmt.Errorf("numeric pre-release identifiers must not contain leading zeroes: %w", ErrInvalidVersion)
			}
			n, _ := strconv.Atoi(seg)
			out[i] = preID{num: n, isNum: true}
		} else {
			out[i] = preID{str: seg}
		}
	}
	return out, true, nil
}

// parseBuildGroup parses the build-metadata capture group. An empty string
// means the group was absent (no build metadata).
func parseBuildGroup(s string) ([]string, bool) {
	if s == "" {
		return nil, false
	}
	return strings.Split(s, "."), true
}

// Major returns the major component.
func (v *Version) Major() int { return v.major }

// Minor returns the minor component.
func (v *Version) Minor() int { return v.minor }

// Patch returns the patch component.
func (v *Version) Patch() int { return v.patch }

// Prerelease returns the pre-release identifier without its leading '-', or the
// empty string if there is no pre-release.
func (v *Version) Prerelease() string {
	if !v.hasPre || len(v.pre) == 0 {
		return ""
	}
	parts := make([]string, len(v.pre))
	for i, p := range v.pre {
		parts[i] = p.String()
	}
	return strings.Join(parts, ".")
}

// Build returns the build-metadata identifier without its leading '+', or the
// empty string if there is no build metadata.
func (v *Version) Build() string {
	if !v.hasBuild || len(v.build) == 0 {
		return ""
	}
	return strings.Join(v.build, ".")
}

// Stable reports whether this is a stable release, i.e. it carries no
// pre-release identifier.
func (v *Version) Stable() bool {
	return !v.hasPre || len(v.pre) == 0
}

// ToStable returns this version stripped of any pre-release identifier. Build
// metadata is preserved. If the version is already stable it is returned
// unchanged.
func (v *Version) ToStable() *Version {
	if !v.hasPre {
		return v
	}
	return &Version{
		major: v.major, minor: v.minor, patch: v.patch,
		hasBuild: v.hasBuild, build: v.build,
	}
}

// NextMajor returns MAJOR+1.0.0.
func (v *Version) NextMajor() *Version {
	return &Version{major: v.major + 1, inf: v.inf}
}

// NextMinor returns MAJOR.MINOR+1.0.
func (v *Version) NextMinor() *Version {
	return &Version{major: v.major, minor: v.minor + 1, inf: v.inf}
}

// NextPatch returns MAJOR.MINOR.PATCH+1.
func (v *Version) NextPatch() *Version {
	return &Version{major: v.major, minor: v.minor, patch: v.patch + 1, inf: v.inf}
}

// String returns the canonical string representation of the version.
func (v *Version) String() string {
	var b strings.Builder
	if v.inf {
		b.WriteString("Infinity")
	} else {
		b.WriteString(strconv.Itoa(v.major))
	}
	b.WriteByte('.')
	b.WriteString(strconv.Itoa(v.minor))
	b.WriteByte('.')
	b.WriteString(strconv.Itoa(v.patch))
	if v.hasPre {
		b.WriteByte('-')
		for i, p := range v.pre {
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(p.String())
		}
	}
	if v.hasBuild {
		b.WriteByte('+')
		b.WriteString(strings.Join(v.build, "."))
	}
	return b.String()
}

// Compare compares v and other by SemVer precedence, returning -1, 0 or +1.
// Build metadata is ignored, as required by the specification.
func (v *Version) Compare(other *Version) int {
	if c := compareMajor(v, other); c != 0 {
		return c
	}
	if c := compareInt(v.minor, other.minor); c != 0 {
		return c
	}
	if c := compareInt(v.patch, other.patch); c != 0 {
		return c
	}
	return comparePrerelease(v, other)
}

func compareMajor(a, b *Version) int {
	switch {
	case a.inf && b.inf:
		return 0
	case a.inf:
		return 1
	case b.inf:
		return -1
	default:
		return compareInt(a.major, b.major)
	}
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePrerelease(a, b *Version) int {
	if !a.hasPre {
		if !b.hasPre {
			return 0
		}
		return 1
	}
	if !b.hasPre {
		return -1
	}
	yourMax := len(b.pre)
	for idx, x := range a.pre {
		if idx >= yourMax {
			return 1
		}
		if c := comparePreID(x, b.pre[idx]); c != 0 {
			return c
		}
	}
	return compareInt(len(a.pre), yourMax)
}

func comparePreID(x, y preID) int {
	if x.isNum {
		if y.isNum {
			return compareInt(x.num, y.num)
		}
		return -1
	}
	if y.isNum {
		return 1
	}
	return strings.Compare(x.str, y.str)
}

// Equal reports whether v and other are identical, including pre-release and
// build metadata (an absent pre-release differs from an empty one).
func (v *Version) Equal(other *Version) bool {
	return v.major == other.major &&
		v.inf == other.inf &&
		v.minor == other.minor &&
		v.patch == other.patch &&
		equalPre(v.pre, v.hasPre, other.pre, other.hasPre) &&
		equalBuild(v.build, v.hasBuild, other.build, other.hasBuild)
}

func equalPre(a []preID, aHas bool, b []preID, bHas bool) bool {
	if aHas != bHas || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].isNum != b[i].isNum || a[i].num != b[i].num || a[i].str != b[i].str {
			return false
		}
	}
	return true
}

func equalBuild(a []string, aHas bool, b []string, bHas bool) bool {
	if aHas != bHas || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// MinVersion returns the lowest-precedence version among versions, or nil if
// no versions are supplied.
func MinVersion(versions ...*Version) *Version {
	if len(versions) == 0 {
		return nil
	}
	m := versions[0]
	for _, v := range versions[1:] {
		if v.Compare(m) < 0 {
			m = v
		}
	}
	return m
}

// MaxVersion returns the highest-precedence version among versions, or nil if
// no versions are supplied.
func MaxVersion(versions ...*Version) *Version {
	if len(versions) == 0 {
		return nil
	}
	m := versions[0]
	for _, v := range versions[1:] {
		if v.Compare(m) > 0 {
			m = v
		}
	}
	return m
}

// SortVersions sorts versions in ascending SemVer precedence order, in place.
func SortVersions(versions []*Version) {
	sort.Sort(ByVersion(versions))
}

// ByVersion attaches [sort.Interface] to a slice of versions, ordering them by
// ascending SemVer precedence.
type ByVersion []*Version

func (s ByVersion) Len() int           { return len(s) }
func (s ByVersion) Less(i, j int) bool { return s[i].Compare(s[j]) < 0 }
func (s ByVersion) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
