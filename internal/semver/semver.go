// Package semver provides semantic version parsing and constraint checking.
package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a semantic version
type Version struct {
	parts      []int // version parts (major, minor, patch, and any additional components)
	prerelease string
	metadata   string
	original   string
}

// NewVersion parses a string into a Version
func NewVersion(v string) (Version, error) {
	original := v
	// Strip leading 'v' if present
	v = strings.TrimPrefix(v, "v")

	// Split on + for metadata
	parts := strings.SplitN(v, "+", 2)
	v = parts[0]
	metadata := ""
	if len(parts) > 1 {
		metadata = parts[1]
	}

	// Split on - for prerelease
	parts = strings.SplitN(v, "-", 2)
	v = parts[0]
	prerelease := ""
	if len(parts) > 1 {
		prerelease = parts[1]
	}

	// Parse version numbers
	versionParts := strings.Split(v, ".")
	if len(versionParts) < 1 {
		return Version{}, fmt.Errorf("invalid version format: %s", original)
	}

	versionNumbers := make([]int, len(versionParts))
	for i, part := range versionParts {
		num, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version part %d: %s", i, original)
		}
		versionNumbers[i] = num
	}

	return Version{
		parts:      versionNumbers,
		prerelease: prerelease,
		metadata:   metadata,
		original:   original,
	}, nil
}

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other
func (v Version) Compare(other Version) int {
	// Compare version parts
	maxLen := max(len(other.parts), len(v.parts))

	for i := range maxLen {
		vPart := 0
		if i < len(v.parts) {
			vPart = v.parts[i]
		}
		oPart := 0
		if i < len(other.parts) {
			oPart = other.parts[i]
		}

		if vPart != oPart {
			if vPart < oPart {
				return -1
			}
			return 1
		}
	}

	// Handle prerelease comparison
	// Version without prerelease is greater than version with prerelease
	if v.prerelease == "" && other.prerelease != "" {
		return 1
	}
	if v.prerelease != "" && other.prerelease == "" {
		return -1
	}

	// Both have prerelease, compare them
	if v.prerelease != other.prerelease {
		return comparePrereleases(v.prerelease, other.prerelease)
	}

	return 0
}

// comparePrereleases compares two prerelease strings according to semver rules
func comparePrereleases(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	// Compare each identifier
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		aCur, bCur := aParts[i], bParts[i]
		for aCur != "" && bCur != "" {
			aHead, aNum, aTail := splitNumeric(aCur)
			bHead, bNum, bTail := splitNumeric(bCur)
			if aHead != bHead {
				if aHead < bHead {
					return -1
				}
				return 1
			}
			aN, aErr := strconv.Atoi(aNum)
			bN, bErr := strconv.Atoi(bNum)
			if aErr != nil || bErr != nil {
				// this should potentially panic
				if bErr == nil {
					return -1
				}
				if aErr == nil {
					return 1
				}
				return 0
			}
			if aN != bN {
				if aN < bN {
					return -1
				}
				return 1
			}
			aCur, bCur = aTail, bTail
		}
		if aCur != "" {
			return 1
		}
		if bCur != "" {
			return -1
		}
	}
	if len(aParts) != len(bParts) {
		if len(aParts) < len(bParts) {
			return -1
		}
		return 1
	}

	return 0
}

// Split an alpha-numeric string into:
//  1. non-numeric head of string
//  2. numeric portion
//  3. tail of string
func splitNumeric(v string) (string, string, string) {
	head := v
	// find the split where the number starts
	for i, c := range v {
		if c >= '0' && c <= '9' {
			head = v[:i]
			break
		}
	}
	num, tail := v[len(head):], ""
	// find the split where the number stops
	for i, c := range num {
		if c < '0' || c > '9' {
			tail = num[i:]
			num = num[:i]
			break
		}
	}
	return head, num, tail
}

// String returns the original version string
func (v Version) String() string {
	return v.original
}

// Major returns the major version number
func (v Version) Major() int {
	if len(v.parts) > 0 {
		return v.parts[0]
	}
	return 0
}

// Minor returns the minor version number
func (v Version) Minor() int {
	if len(v.parts) > 1 {
		return v.parts[1]
	}
	return 0
}

// Patch returns the patch version number
func (v Version) Patch() int {
	if len(v.parts) > 2 {
		return v.parts[2]
	}
	return 0
}

// Constraint represents a version constraint
type Constraint struct {
	constraints []constraint
}

type constraint struct {
	operator string
	version  Version
}

// NewConstraint parses a constraint string
// Supports: >=, <=, >, <, =, ^, ~, and ranges like ">=1.0.0 <2.0.0"
func NewConstraint(c string) (Constraint, error) {
	c = strings.TrimSpace(c)
	if c == "" {
		return Constraint{}, fmt.Errorf("empty constraint")
	}

	// Split on spaces to handle ranges
	parts := strings.Fields(c)
	constraints := []constraint{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle caret (^) constraint
		if after, ok := strings.CutPrefix(part, "^"); ok {
			v, err := NewVersion(after)
			if err != nil {
				return Constraint{}, fmt.Errorf("invalid caret constraint version: %w", err)
			}
			// ^1.2.3 means >=1.2.3 <2.0.0
			// ^0.2.3 means >=0.2.3 <0.3.0
			// ^0.0.3 means >=0.0.3 <0.0.4
			constraints = append(constraints, constraint{operator: ">=", version: v})

			var upperBound Version
			if v.Major() > 0 {
				upperBound = Version{parts: []int{v.Major() + 1, 0, 0}}
			} else if v.Minor() > 0 {
				upperBound = Version{parts: []int{0, v.Minor() + 1, 0}}
			} else {
				upperBound = Version{parts: []int{0, 0, v.Patch() + 1}}
			}
			constraints = append(constraints, constraint{operator: "<", version: upperBound})
			continue
		}

		// Handle tilde (~) constraint
		if after, ok := strings.CutPrefix(part, "~"); ok {
			v, err := NewVersion(after)
			if err != nil {
				return Constraint{}, fmt.Errorf("invalid tilde constraint version: %w", err)
			}
			// ~1.2.3 means >=1.2.3 <1.3.0
			constraints = append(constraints, constraint{operator: ">=", version: v})
			upperBound := Version{parts: []int{v.Major(), v.Minor() + 1, 0}}
			constraints = append(constraints, constraint{operator: "<", version: upperBound})
			continue
		}

		// Handle comparison operators
		op := ""
		vStr := part

		if strings.HasPrefix(part, ">=") {
			op = ">="
			vStr = strings.TrimPrefix(part, ">=")
		} else if strings.HasPrefix(part, "<=") {
			op = "<="
			vStr = strings.TrimPrefix(part, "<=")
		} else if strings.HasPrefix(part, ">") {
			op = ">"
			vStr = strings.TrimPrefix(part, ">")
		} else if strings.HasPrefix(part, "<") {
			op = "<"
			vStr = strings.TrimPrefix(part, "<")
		} else if strings.HasPrefix(part, "=") {
			op = "="
			vStr = strings.TrimPrefix(part, "=")
		} else {
			// No operator means exact match
			op = "="
		}

		v, err := NewVersion(vStr)
		if err != nil {
			return Constraint{}, fmt.Errorf("invalid constraint version: %w", err)
		}

		constraints = append(constraints, constraint{operator: op, version: v})
	}

	if len(constraints) == 0 {
		return Constraint{}, fmt.Errorf("no valid constraints found")
	}

	return Constraint{constraints: constraints}, nil
}

// Check returns true if the version satisfies all constraints
func (c Constraint) Check(v Version) bool {
	for _, con := range c.constraints {
		cmp := v.Compare(con.version)

		switch con.operator {
		case "=":
			if cmp != 0 {
				return false
			}
		case ">":
			if cmp <= 0 {
				return false
			}
		case ">=":
			if cmp < 0 {
				return false
			}
		case "<":
			if cmp >= 0 {
				return false
			}
		case "<=":
			if cmp > 0 {
				return false
			}
		}
	}

	return true
}
