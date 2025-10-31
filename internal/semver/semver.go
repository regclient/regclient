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
	maxLen := len(v.parts)
	if len(other.parts) > maxLen {
		maxLen = len(other.parts)
	}

	for i := 0; i < maxLen; i++ {
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
		aNum, aIsNum := strconv.Atoi(aParts[i])
		bNum, bIsNum := strconv.Atoi(bParts[i])

		// Both numeric: compare as integers
		if aIsNum == nil && bIsNum == nil {
			if aNum != bNum {
				if aNum < bNum {
					return -1
				}
				return 1
			}
			continue
		}

		// Numeric has lower precedence than alphanumeric
		if aIsNum == nil && bIsNum != nil {
			return -1
		}
		if aIsNum != nil && bIsNum == nil {
			return 1
		}

		// Both alphanumeric: lexical comparison
		if aParts[i] != bParts[i] {
			if aParts[i] < bParts[i] {
				return -1
			}
			return 1
		}
	}

	// Fewer parts = lower precedence
	if len(aParts) < len(bParts) {
		return -1
	}
	if len(aParts) > len(bParts) {
		return 1
	}

	return 0
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
		if strings.HasPrefix(part, "^") {
			v, err := NewVersion(strings.TrimPrefix(part, "^"))
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
		if strings.HasPrefix(part, "~") {
			v, err := NewVersion(strings.TrimPrefix(part, "~"))
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
