// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-semantic-puppet/semantic-puppet authors

package semanticpuppet

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ErrInvalidRange is returned (wrapped) by [ParseRange] when a string is not a
// valid version range. Use [errors.Is] to test for it.
var ErrInvalidRange = errors.New("invalid version range")

// ParseError describes a failure to parse a version or version range. It wraps
// either [ErrInvalidVersion] or [ErrInvalidRange].
type ParseError struct {
	// Kind is "version" or "range".
	Kind string
	// Input is the offending string.
	Input string
	err   error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("semanticpuppet: cannot parse %s %q: %v", e.Kind, e.Input, e.err)
}

// Unwrap returns the sentinel error ([ErrInvalidVersion] or [ErrInvalidRange]).
func (e *ParseError) Unwrap() error { return e.err }

// ---------------------------------------------------------------------------
// Range grammar (mirrors semantic_puppet, which follows node-semver).
// ---------------------------------------------------------------------------

const (
	reNR       = `0|[1-9][0-9]*`
	reXR       = `(x|X|\*|` + reNR + `)`
	reXRnc     = `(?:x|X|\*|` + reNR + `)`
	rePart     = `(?:[0-9A-Za-z-]+)`
	reParts    = rePart + `(?:\.` + rePart + `)*`
	reQual     = `(?:-(` + reParts + `))?(?:\+(` + reParts + `))?`
	reQualNC   = `(?:-` + reParts + `)?(?:\+` + reParts + `)?`
	rePartial  = reXRnc + `(?:\.` + reXRnc + `(?:\.` + reXRnc + reQualNC + `)?)?`
	reSimple   = `((?:<=|>=|~>|~=)|[<>=~^])?(` + rePartial + `)`
	reHyphen   = `(` + rePartial + `)\s+-\s+(` + rePartial + `)`
	rePartExpr = reXR + `(?:\.` + reXR + `(?:\.` + reXR + reQual + `)?)?`
)

var (
	simpleRegex     = regexp.MustCompile(`\A` + reSimple + `\z`)
	hyphenRegex     = regexp.MustCompile(`\A` + reHyphen + `\z`)
	partialRegex    = regexp.MustCompile(`\A` + rePartExpr + `\z`)
	logicalOrRegex  = regexp.MustCompile(`\s*\|\|\s*`)
	rangeSplitRegex = regexp.MustCompile(`\s+`)
	// opWsRegex removes whitespace (and an optional leading 'v') after an
	// operator so that such whitespace does not cause a split.
	opWsRegex = regexp.MustCompile(`([><=~^])(?:\s+|\s*v)`)
)

// ---------------------------------------------------------------------------
// Range matchers.
// ---------------------------------------------------------------------------

// abstractRange is one clause of a VersionRange. The concrete implementations
// mirror the private matcher classes of semantic_puppet.
type abstractRange interface {
	include(v *Version) bool
	lower() *Version
	upper() *Version
	excludeBegin() bool
	excludeEnd() bool
	lowerBound() bool
	upperBound() bool
	testPrerelease(v *Version) bool
	intersect(o abstractRange) abstractRange
	merge(o abstractRange) abstractRange
	eq(o abstractRange) bool
	String() string
}

// allRange matches every version.
type allRange struct{}

func (allRange) include(*Version) bool                     { return true }
func (allRange) lower() *Version                           { return versionMin }
func (allRange) upper() *Version                           { return versionMax }
func (allRange) excludeBegin() bool                        { return false }
func (allRange) excludeEnd() bool                          { return false }
func (allRange) lowerBound() bool                          { return false }
func (allRange) upperBound() bool                          { return false }
func (allRange) testPrerelease(*Version) bool              { return true }
func (a allRange) intersect(o abstractRange) abstractRange { return o }
func (a allRange) merge(abstractRange) abstractRange       { return a }
func (allRange) eq(o abstractRange) bool                   { _, ok := o.(allRange); return ok }
func (allRange) String() string                            { return "*" }

var allRangeSingleton abstractRange = allRange{}

// minMaxRange is the intersection of a lower and an upper matcher.
type minMaxRange struct {
	min abstractRange
	max abstractRange
}

func newMinMax(min, max abstractRange) *minMaxRange {
	if m, ok := min.(*minMaxRange); ok {
		min = m.min
	}
	if m, ok := max.(*minMaxRange); ok {
		max = m.max
	}
	return &minMaxRange{min: min, max: max}
}

func (r *minMaxRange) include(v *Version) bool { return r.min.include(v) && r.max.include(v) }
func (r *minMaxRange) lower() *Version         { return r.min.lower() }
func (r *minMaxRange) upper() *Version         { return r.max.upper() }
func (r *minMaxRange) excludeBegin() bool      { return r.min.excludeBegin() }
func (r *minMaxRange) excludeEnd() bool        { return r.max.excludeEnd() }
func (r *minMaxRange) lowerBound() bool        { return r.min.lowerBound() }
func (r *minMaxRange) upperBound() bool        { return r.max.upperBound() }
func (r *minMaxRange) testPrerelease(v *Version) bool {
	return r.min.testPrerelease(v) || r.max.testPrerelease(v)
}
func (r *minMaxRange) intersect(o abstractRange) abstractRange { return genericIntersect(r, o) }
func (r *minMaxRange) merge(o abstractRange) abstractRange     { return genericMerge(r, o) }
func (r *minMaxRange) eq(o abstractRange) bool {
	other, ok := o.(*minMaxRange)
	return ok && r.min.eq(other.min) && r.max.eq(other.max)
}
func (r *minMaxRange) String() string { return r.min.String() + " " + r.max.String() }

// gtRange matches versions strictly greater than version.
type gtRange struct{ version *Version }

func (r *gtRange) include(v *Version) bool                 { return v.Compare(r.version) > 0 }
func (r *gtRange) lower() *Version                         { return r.version }
func (r *gtRange) upper() *Version                         { return versionMax }
func (r *gtRange) excludeBegin() bool                      { return true }
func (r *gtRange) excludeEnd() bool                        { return false }
func (r *gtRange) lowerBound() bool                        { return true }
func (r *gtRange) upperBound() bool                        { return false }
func (r *gtRange) testPrerelease(v *Version) bool          { return comparatorTestPrerelease(r.version, v) }
func (r *gtRange) intersect(o abstractRange) abstractRange { return genericIntersect(r, o) }
func (r *gtRange) merge(o abstractRange) abstractRange     { return genericMerge(r, o) }
func (r *gtRange) eq(o abstractRange) bool {
	other, ok := o.(*gtRange)
	return ok && r.version.Equal(other.version)
}
func (r *gtRange) String() string { return ">" + r.version.String() }

// gtEqRange matches versions greater than or equal to version.
type gtEqRange struct{ version *Version }

func (r *gtEqRange) include(v *Version) bool                 { return v.Compare(r.version) >= 0 }
func (r *gtEqRange) lower() *Version                         { return r.version }
func (r *gtEqRange) upper() *Version                         { return versionMax }
func (r *gtEqRange) excludeBegin() bool                      { return false }
func (r *gtEqRange) excludeEnd() bool                        { return false }
func (r *gtEqRange) lowerBound() bool                        { return !r.version.Equal(versionMin) }
func (r *gtEqRange) upperBound() bool                        { return false }
func (r *gtEqRange) testPrerelease(v *Version) bool          { return comparatorTestPrerelease(r.version, v) }
func (r *gtEqRange) intersect(o abstractRange) abstractRange { return genericIntersect(r, o) }
func (r *gtEqRange) merge(o abstractRange) abstractRange     { return genericMerge(r, o) }
func (r *gtEqRange) eq(o abstractRange) bool {
	other, ok := o.(*gtEqRange)
	return ok && r.version.Equal(other.version)
}
func (r *gtEqRange) String() string { return ">=" + r.version.String() }

// ltRange matches versions strictly less than version.
type ltRange struct{ version *Version }

func (r *ltRange) include(v *Version) bool                 { return v.Compare(r.version) < 0 }
func (r *ltRange) lower() *Version                         { return versionMin }
func (r *ltRange) upper() *Version                         { return r.version }
func (r *ltRange) excludeBegin() bool                      { return false }
func (r *ltRange) excludeEnd() bool                        { return true }
func (r *ltRange) lowerBound() bool                        { return false }
func (r *ltRange) upperBound() bool                        { return true }
func (r *ltRange) testPrerelease(v *Version) bool          { return comparatorTestPrerelease(r.version, v) }
func (r *ltRange) intersect(o abstractRange) abstractRange { return genericIntersect(r, o) }
func (r *ltRange) merge(o abstractRange) abstractRange     { return genericMerge(r, o) }
func (r *ltRange) eq(o abstractRange) bool {
	other, ok := o.(*ltRange)
	return ok && r.version.Equal(other.version)
}
func (r *ltRange) String() string {
	if r == matchNothing {
		return "<0.0.0"
	}
	return "<" + r.version.String()
}

// ltEqRange matches versions less than or equal to version.
type ltEqRange struct{ version *Version }

func (r *ltEqRange) include(v *Version) bool                 { return v.Compare(r.version) <= 0 }
func (r *ltEqRange) lower() *Version                         { return versionMin }
func (r *ltEqRange) upper() *Version                         { return r.version }
func (r *ltEqRange) excludeBegin() bool                      { return false }
func (r *ltEqRange) excludeEnd() bool                        { return false }
func (r *ltEqRange) lowerBound() bool                        { return false }
func (r *ltEqRange) upperBound() bool                        { return !r.version.Equal(versionMax) }
func (r *ltEqRange) testPrerelease(v *Version) bool          { return comparatorTestPrerelease(r.version, v) }
func (r *ltEqRange) intersect(o abstractRange) abstractRange { return genericIntersect(r, o) }
func (r *ltEqRange) merge(o abstractRange) abstractRange     { return genericMerge(r, o) }
func (r *ltEqRange) eq(o abstractRange) bool {
	other, ok := o.(*ltEqRange)
	return ok && r.version.Equal(other.version)
}
func (r *ltEqRange) String() string { return "<=" + r.version.String() }

// eqRange matches exactly one version.
type eqRange struct{ version *Version }

func (r *eqRange) include(v *Version) bool                 { return v.Equal(r.version) }
func (r *eqRange) lower() *Version                         { return r.version }
func (r *eqRange) upper() *Version                         { return r.version }
func (r *eqRange) excludeBegin() bool                      { return false }
func (r *eqRange) excludeEnd() bool                        { return false }
func (r *eqRange) lowerBound() bool                        { return !r.version.Equal(versionMin) }
func (r *eqRange) upperBound() bool                        { return !r.version.Equal(versionMax) }
func (r *eqRange) testPrerelease(v *Version) bool          { return comparatorTestPrerelease(r.version, v) }
func (r *eqRange) intersect(o abstractRange) abstractRange { return genericIntersect(r, o) }
func (r *eqRange) merge(o abstractRange) abstractRange     { return genericMerge(r, o) }
func (r *eqRange) eq(o abstractRange) bool {
	other, ok := o.(*eqRange)
	return ok && r.version.Equal(other.version)
}
func (r *eqRange) String() string { return r.version.String() }

// matchNothing is the canonical empty matcher (<0.0.0).
var matchNothing = &ltRange{version: versionMin}

// comparatorTestPrerelease reports whether cmpVersion (the version carried by a
// comparator matcher) explicitly authorises a pre-release sharing the same
// major.minor.patch triple as v.
func comparatorTestPrerelease(cmpVersion, v *Version) bool {
	return !cmpVersion.Stable() &&
		cmpVersion.major == v.major &&
		cmpVersion.minor == v.minor &&
		cmpVersion.patch == v.patch
}

// genericIntersect computes the intersection of two matchers, or nil when they
// do not overlap.
func genericIntersect(self, rng abstractRange) abstractRange {
	cmp := self.lower().Compare(rng.upper())
	if cmp > 0 {
		return nil
	}
	if cmp == 0 {
		if self.excludeBegin() || rng.excludeEnd() {
			return nil
		}
		return &eqRange{self.lower()}
	}
	cmp = rng.lower().Compare(self.upper())
	if cmp > 0 {
		return nil
	}
	if cmp == 0 {
		if rng.excludeBegin() || self.excludeEnd() {
			return nil
		}
		return &eqRange{rng.lower()}
	}

	var min abstractRange
	switch cmp = self.lower().Compare(rng.lower()); {
	case cmp < 0:
		min = rng
	case cmp > 0:
		min = self
	default:
		if self.excludeBegin() {
			min = self
		} else {
			min = rng
		}
	}

	var max abstractRange
	switch cmp = self.upper().Compare(rng.upper()); {
	case cmp > 0:
		max = rng
	case cmp < 0:
		max = self
	default:
		if self.excludeEnd() {
			max = self
		} else {
			max = rng
		}
	}

	if !max.upperBound() {
		return min
	}
	if !min.lowerBound() {
		return max
	}
	return newMinMax(min, max)
}

// genericMerge merges two matchers into one covering their union, or nil when
// they are neither overlapping nor adjacent.
func genericMerge(self, other abstractRange) abstractRange {
	if self.include(other.lower()) || other.include(self.lower()) {
		var min *Version
		var exclBegin bool
		switch self.lower().Compare(other.lower()) {
		case -1:
			min, exclBegin = self.lower(), self.excludeBegin()
		case 1:
			min, exclBegin = other.lower(), other.excludeBegin()
		default:
			min, exclBegin = self.lower(), self.excludeBegin() && other.excludeBegin()
		}

		var max *Version
		var exclEnd bool
		switch self.upper().Compare(other.upper()) {
		case 1:
			max, exclEnd = self.upper(), self.excludeEnd()
		case -1:
			max, exclEnd = other.upper(), other.excludeEnd()
		default:
			max, exclEnd = self.upper(), self.excludeEnd() && other.excludeEnd()
		}

		var lb, ub abstractRange
		if exclBegin {
			lb = &gtRange{min}
		} else {
			lb = &gtEqRange{min}
		}
		if exclEnd {
			ub = &ltRange{max}
		} else {
			ub = &ltEqRange{max}
		}
		return createMinMax(lb, ub)
	}
	switch {
	case self.excludeEnd() && !other.excludeBegin() && self.upper().Equal(other.lower()):
		return fromTo(self, other)
	case other.excludeEnd() && !self.excludeBegin() && other.upper().Equal(self.lower()):
		return fromTo(other, self)
	case !self.excludeEnd() && !other.excludeBegin() && self.upper().NextPatch().Equal(other.lower()):
		return fromTo(self, other)
	case !other.excludeEnd() && !self.excludeBegin() && other.upper().NextPatch().Equal(self.lower()):
		return fromTo(other, self)
	default:
		return nil
	}
}

func fromTo(a, b abstractRange) abstractRange {
	var lb, ub abstractRange
	if a.excludeBegin() {
		lb = &gtRange{a.lower()}
	} else {
		lb = &gtEqRange{a.lower()}
	}
	if b.excludeEnd() {
		ub = &ltRange{b.upper()}
	} else {
		ub = &ltEqRange{b.upper()}
	}
	return createMinMax(lb, ub)
}

// createMinMax reduces a set of matchers with intersection, mirroring
// MinMaxRange.create. It returns nil for an empty set or when any intermediate
// intersection is empty.
func createMinMax(ranges ...abstractRange) abstractRange {
	var memo abstractRange
	for i, r := range ranges {
		if i == 0 {
			memo = r
			continue
		}
		if memo == nil {
			return nil
		}
		memo = memo.intersect(r)
	}
	return memo
}

// ---------------------------------------------------------------------------
// Range-parsing helpers (mirror the private class methods of semantic_puppet).
// ---------------------------------------------------------------------------

// digit interprets an XR capture group. It returns ok=false for an absent
// group or an x-range wildcard (x, X, *).
func digit(s string) (int, bool) {
	switch s {
	case "", "x", "X", "*":
		return 0, false
	default:
		n, _ := strconv.Atoi(s)
		return n, true
	}
}

func digitOrZero(s string) int {
	n, _ := digit(s)
	return n
}

func parsePartial(expr string) ([]string, error) {
	m := partialRegex.FindStringSubmatch(expr)
	if m == nil {
		return nil, ErrInvalidRange
	}
	return m, nil
}

func versionFromParts(major, minor, patch int, m []string) (*Version, error) {
	pre, hasPre, err := parsePrereleaseGroup(m[4])
	if err != nil {
		return nil, err
	}
	build, hasBuild := parseBuildGroup(m[5])
	return &Version{
		major: major, minor: minor, patch: patch,
		hasPre: hasPre, pre: pre,
		hasBuild: hasBuild, build: build,
	}, nil
}

func parseVersion(expr string) (*Version, error) {
	m, err := parsePartial(expr)
	if err != nil {
		return nil, err
	}
	return versionFromParts(digitOrZero(m[1]), digitOrZero(m[2]), digitOrZero(m[3]), m)
}

func parseCaret(expr string) (abstractRange, error) {
	m, err := parsePartial(expr)
	if err != nil {
		return nil, err
	}
	major, ok := digit(m[1])
	if ok && major == 0 {
		return allowPatchUpdates(major, ok, m, true)
	}
	return allowMinorUpdates(major, ok, m)
}

func parseTilde(expr string) (abstractRange, error) {
	m, err := parsePartial(expr)
	if err != nil {
		return nil, err
	}
	major, ok := digit(m[1])
	return allowPatchUpdates(major, ok, m, true)
}

func parseXRange(expr string) (abstractRange, error) {
	m, err := parsePartial(expr)
	if err != nil {
		return nil, err
	}
	major, ok := digit(m[1])
	return allowPatchUpdates(major, ok, m, false)
}

func allowPatchUpdates(major int, majorOK bool, m []string, tildeOrCaret bool) (abstractRange, error) {
	if !majorOK {
		return allRangeSingleton, nil
	}
	minor, minorOK := digit(m[2])
	if !minorOK {
		return newMinMax(&gtEqRange{&Version{major: major}}, &ltRange{&Version{major: major + 1}}), nil
	}
	patch, patchOK := digit(m[3])
	if !patchOK {
		return newMinMax(&gtEqRange{&Version{major: major, minor: minor}}, &ltRange{&Version{major: major, minor: minor + 1}}), nil
	}
	v, err := versionFromParts(major, minor, patch, m)
	if err != nil {
		return nil, err
	}
	if !tildeOrCaret {
		return &eqRange{v}, nil
	}
	return newMinMax(&gtEqRange{v}, &ltRange{&Version{major: major, minor: minor + 1}}), nil
}

func allowMinorUpdates(major int, majorOK bool, m []string) (abstractRange, error) {
	if !majorOK {
		return allRangeSingleton, nil
	}
	minor, minorOK := digit(m[2])
	if !minorOK {
		return newMinMax(&gtEqRange{&Version{major: major}}, &ltRange{&Version{major: major + 1}}), nil
	}
	patch, patchOK := digit(m[3])
	if !patchOK {
		return newMinMax(&gtEqRange{&Version{major: major, minor: minor}}, &ltRange{&Version{major: major + 1}}), nil
	}
	if m[4] == "" {
		return newMinMax(&gtEqRange{&Version{major: major, minor: minor, patch: patch}}, &ltRange{&Version{major: major + 1}}), nil
	}
	v, err := versionFromParts(major, minor, patch, m)
	if err != nil {
		return nil, err
	}
	return newMinMax(&gtEqRange{v}, &ltRange{&Version{major: major + 1}}), nil
}

func parseGtVersion(expr string) (abstractRange, error) {
	m, err := parsePartial(expr)
	if err != nil {
		return nil, err
	}
	major, ok := digit(m[1])
	if !ok {
		return matchNothing, nil
	}
	minor, ok := digit(m[2])
	if !ok {
		return &gtEqRange{&Version{major: major + 1}}, nil
	}
	patch, ok := digit(m[3])
	if !ok {
		return &gtEqRange{&Version{major: major, minor: minor + 1}}, nil
	}
	v, err := versionFromParts(major, minor, patch, m)
	if err != nil {
		return nil, err
	}
	return &gtRange{v}, nil
}

func parseLtEqVersion(expr string) (abstractRange, error) {
	m, err := parsePartial(expr)
	if err != nil {
		return nil, err
	}
	major, ok := digit(m[1])
	if !ok {
		return allRangeSingleton, nil
	}
	minor, ok := digit(m[2])
	if !ok {
		return &ltRange{&Version{major: major + 1}}, nil
	}
	patch, ok := digit(m[3])
	if !ok {
		return &ltRange{&Version{major: major, minor: minor + 1}}, nil
	}
	v, err := versionFromParts(major, minor, patch, m)
	if err != nil {
		return nil, err
	}
	return &ltEqRange{v}, nil
}

// ---------------------------------------------------------------------------
// VersionRange.
// ---------------------------------------------------------------------------

// VersionRange is an immutable set of versions expressed as a union of
// comparator clauses. Construct one with [ParseRange].
type VersionRange struct {
	ranges []abstractRange
	str    string
}

// allRangeVR matches every version and is returned for an empty range string.
var allRangeVR = &VersionRange{ranges: []abstractRange{allRangeSingleton}, str: "*"}

// emptyRange matches no version.
var emptyRange = newRangeList(nil, nil)

// ParseRange parses a version-range string. It accepts plain versions
// ("1.2.3"), the comparators <, <=, >, >=, =, hyphen ranges ("1.0.0 - 2.0.0"),
// the tilde (~, ~>, ~=) and caret (^) operators, x-ranges ("1.x", "1.2.x",
// "*") and logical-or unions ("... || ..."). An empty string matches every
// version. It returns an error wrapping [ErrInvalidRange] for malformed input.
func ParseRange(rangeString string) (*VersionRange, error) {
	rangeSet := opWsRegex.ReplaceAllString(rangeString, "${1}")
	ranges := rubySplit(logicalOrRegex, rangeSet)
	if len(ranges) == 0 {
		return allRangeVR, nil
	}

	elems := make([]abstractRange, 0, len(ranges))
	for _, rng := range ranges {
		if hm := hyphenRegex.FindStringSubmatch(rng); hm != nil {
			lo, err := parseVersion(hm[1])
			if err != nil {
				return nil, &ParseError{Kind: "range", Input: rangeString, err: err}
			}
			hi, err := parseVersion(hm[2])
			if err != nil {
				return nil, &ParseError{Kind: "range", Input: rangeString, err: err}
			}
			elems = append(elems, createMinMax(&gtEqRange{lo}, &ltEqRange{hi}))
			continue
		}

		simples := make([]abstractRange, 0)
		for _, simple := range rubySplit(rangeSplitRegex, rng) {
			ar, err := parseSimple(simple)
			if err != nil {
				return nil, &ParseError{Kind: "range", Input: rangeString, err: err}
			}
			simples = append(simples, ar)
		}
		if len(simples) == 1 {
			elems = append(elems, simples[0])
		} else {
			elems = append(elems, createMinMax(simples...))
		}
	}

	elems = uniqRanges(elems)
	return newRangeList(elems, &rangeString), nil
}

// MustParseRange is like [ParseRange] but panics on error. It is intended for
// use with constant range strings.
func MustParseRange(rangeString string) *VersionRange {
	r, err := ParseRange(rangeString)
	if err != nil {
		panic(err)
	}
	return r
}

func parseSimple(simple string) (abstractRange, error) {
	m := simpleRegex.FindStringSubmatch(simple)
	if m == nil {
		return nil, ErrInvalidRange
	}
	operand := m[2]
	switch m[1] {
	case "~", "~>", "~=":
		return parseTilde(operand)
	case "^":
		return parseCaret(operand)
	case ">":
		return parseGtVersion(operand)
	case ">=":
		v, err := parseVersion(operand)
		if err != nil {
			return nil, err
		}
		return &gtEqRange{v}, nil
	case "<":
		v, err := parseVersion(operand)
		if err != nil {
			return nil, err
		}
		return &ltRange{v}, nil
	case "<=":
		return parseLtEqVersion(operand)
	default:
		return parseXRange(operand)
	}
}

// newRangeList builds a VersionRange from a set of matchers, compacting nils,
// merging overlapping/adjacent clauses and defaulting to matchNothing when the
// set is empty. When str is nil the canonical inspect form is used.
func newRangeList(ranges []abstractRange, str *string) *VersionRange {
	ranges = compactRanges(ranges)
	ranges = mergeRanges(ranges)
	if len(ranges) == 0 {
		ranges = []abstractRange{matchNothing}
	}
	var s string
	if str != nil {
		s = *str
	} else {
		s = joinRanges(ranges)
	}
	return &VersionRange{ranges: ranges, str: s}
}

func mergeRanges(ranges []abstractRange) []abstractRange {
	mergeHappened := true
	for len(ranges) > 1 && mergeHappened {
		mergeHappened = false
		var result []abstractRange
		for len(ranges) > 0 {
			var unmerged []abstractRange
			memo := ranges[len(ranges)-1]
			ranges = ranges[:len(ranges)-1]
			for _, y := range ranges {
				merged := memo.merge(y)
				if merged == nil {
					unmerged = append(unmerged, y)
				} else {
					mergeHappened = true
					memo = merged
				}
			}
			result = append(result, memo)
			ranges = unmerged
		}
		for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
			result[i], result[j] = result[j], result[i]
		}
		ranges = result
	}
	return ranges
}

func compactRanges(in []abstractRange) []abstractRange {
	out := in[:0]
	for _, r := range in {
		if r != nil {
			out = append(out, r)
		}
	}
	return out
}

func uniqRanges(in []abstractRange) []abstractRange {
	var out []abstractRange
	for _, x := range in {
		dup := false
		for _, y := range out {
			if rangeEqual(x, y) {
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

func rangeEqual(a, b abstractRange) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.eq(b)
}

func joinRanges(ranges []abstractRange) string {
	parts := make([]string, len(ranges))
	for i, r := range ranges {
		parts[i] = r.String()
	}
	return strings.Join(parts, " || ")
}

// rubySplit mirrors Ruby's String#split(regex): it keeps leading and interior
// empty fields but drops trailing empty fields (so "" yields no fields).
func rubySplit(re *regexp.Regexp, s string) []string {
	parts := re.Split(s, -1)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// Include reports whether version is a member of the range. A pre-release
// version is only matched when a clause of the range explicitly names a
// pre-release sharing the same major.minor.patch triple.
func (r *VersionRange) Include(version *Version) bool {
	for _, rng := range r.ranges {
		if rng.include(version) && (version.Stable() || rng.testPrerelease(version)) {
			return true
		}
	}
	return false
}

// Cover is an alias for [VersionRange.Include].
func (r *VersionRange) Cover(version *Version) bool { return r.Include(version) }

// Intersection returns the range covering exactly the versions matched by both
// r and other. If the ranges do not overlap, an empty range is returned.
func (r *VersionRange) Intersection(other *VersionRange) *VersionRange {
	var result []abstractRange
	for _, a := range r.ranges {
		for _, b := range other.ranges {
			if x := a.intersect(b); x != nil {
				result = append(result, x)
			}
		}
	}
	result = uniqRanges(result)
	if len(result) == 0 {
		return emptyRange
	}
	return newRangeList(result, nil)
}

// Min returns the version that begins the range, or nil if the range is a
// union of more than one disjoint clause.
func (r *VersionRange) Min() *Version {
	if len(r.ranges) == 1 {
		return r.ranges[0].lower()
	}
	return nil
}

// Max returns the version that ends the range, or nil if the range is a union
// of more than one disjoint clause.
func (r *VersionRange) Max() *Version {
	if len(r.ranges) == 1 {
		return r.ranges[0].upper()
	}
	return nil
}

// Equal reports whether r and other consist of the same matcher clauses.
func (r *VersionRange) Equal(other *VersionRange) bool {
	if len(r.ranges) != len(other.ranges) {
		return false
	}
	for i := range r.ranges {
		if !r.ranges[i].eq(other.ranges[i]) {
			return false
		}
	}
	return true
}

// String returns the string the range was parsed from, or the canonical
// inspect form for ranges produced by intersection.
func (r *VersionRange) String() string { return r.str }

// Inspect returns the canonical representation of the range assembled from its
// matcher clauses, independent of how it was originally written.
func (r *VersionRange) Inspect() string { return joinRanges(r.ranges) }
