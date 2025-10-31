package semver

import (
	"testing"
)

func TestNewVersion(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		major       int
		minor       int
		patch       int
		prerelease  string
	}{
		{name: "simple version", input: "1.2.3", major: 1, minor: 2, patch: 3},
		{name: "version with v prefix", input: "v1.2.3", major: 1, minor: 2, patch: 3},
		{name: "version with prerelease", input: "1.2.3-rc1", major: 1, minor: 2, patch: 3, prerelease: "rc1"},
		{name: "version with v and prerelease", input: "v1.2.3-beta.1", major: 1, minor: 2, patch: 3, prerelease: "beta.1"},
		{name: "version with metadata", input: "1.2.3+build123", major: 1, minor: 2, patch: 3},
		{name: "major only", input: "1", major: 1, minor: 0, patch: 0},
		{name: "major.minor", input: "1.2", major: 1, minor: 2, patch: 0},
		{name: "zero version", input: "0.0.0", major: 0, minor: 0, patch: 0},
		{name: "windows version", input: "10.0.17763.2114", major: 10, minor: 0, patch: 17763},
		{name: "invalid format", input: "abc", expectError: true},
		{name: "empty string", input: "", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewVersion(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if v.Major() != tt.major {
				t.Errorf("major: expected %d, got %d", tt.major, v.Major())
			}
			if v.Minor() != tt.minor {
				t.Errorf("minor: expected %d, got %d", tt.minor, v.Minor())
			}
			if v.Patch() != tt.patch {
				t.Errorf("patch: expected %d, got %d", tt.patch, v.Patch())
			}
			if v.prerelease != tt.prerelease {
				t.Errorf("prerelease: expected %q, got %q", tt.prerelease, v.prerelease)
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{name: "equal", v1: "1.2.3", v2: "1.2.3", expected: 0},
		{name: "major less", v1: "1.2.3", v2: "2.2.3", expected: -1},
		{name: "major greater", v1: "2.2.3", v2: "1.2.3", expected: 1},
		{name: "minor less", v1: "1.2.3", v2: "1.3.3", expected: -1},
		{name: "minor greater", v1: "1.3.3", v2: "1.2.3", expected: 1},
		{name: "patch less", v1: "1.2.3", v2: "1.2.4", expected: -1},
		{name: "patch greater", v1: "1.2.4", v2: "1.2.3", expected: 1},
		{name: "with v prefix", v1: "v1.2.3", v2: "v1.2.3", expected: 0},
		{name: "prerelease less than release", v1: "1.2.3-rc1", v2: "1.2.3", expected: -1},
		{name: "release greater than prerelease", v1: "1.2.3", v2: "1.2.3-rc1", expected: 1},
		{name: "prerelease comparison", v1: "1.2.3-alpha", v2: "1.2.3-beta", expected: -1},
		{name: "windows version equal", v1: "10.0.17763.2114", v2: "10.0.17763.2114", expected: 0},
		{name: "windows version less fourth part", v1: "10.0.17763.2114", v2: "10.0.17763.2115", expected: -1},
		{name: "windows version greater fourth part", v1: "10.0.17763.2115", v2: "10.0.17763.2114", expected: 1},
		{name: "windows vs semver", v1: "10.0.17763", v2: "10.0.17763.0", expected: 0},
		{name: "different length shorter less", v1: "1.2.3", v2: "1.2.3.1", expected: -1},
		{name: "different length longer greater", v1: "1.2.3.1", v2: "1.2.3", expected: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v1, err := NewVersion(tt.v1)
			if err != nil {
				t.Fatalf("failed to parse v1: %v", err)
			}
			v2, err := NewVersion(tt.v2)
			if err != nil {
				t.Fatalf("failed to parse v2: %v", err)
			}
			result := v1.Compare(v2)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestNewConstraint(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{name: "simple >=", input: ">=1.0.0", expectError: false},
		{name: "simple <=", input: "<=2.0.0", expectError: false},
		{name: "simple >", input: ">1.0.0", expectError: false},
		{name: "simple <", input: "<2.0.0", expectError: false},
		{name: "simple =", input: "=1.0.0", expectError: false},
		{name: "range", input: ">=1.0.0 <2.0.0", expectError: false},
		{name: "caret", input: "^1.2.3", expectError: false},
		{name: "tilde", input: "~1.2.3", expectError: false},
		{name: "empty", input: "", expectError: true},
		{name: "invalid version", input: ">=abc", expectError: true},
		{name: "invalid range format", input: "invalid range", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewConstraint(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConstraintCheck(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string
		expected   bool
	}{
		// Basic comparisons
		{name: ">= pass", constraint: ">=1.0.0", version: "1.0.0", expected: true},
		{name: ">= pass higher", constraint: ">=1.0.0", version: "1.5.0", expected: true},
		{name: ">= fail", constraint: ">=1.0.0", version: "0.9.0", expected: false},
		{name: "<= pass", constraint: "<=2.0.0", version: "2.0.0", expected: true},
		{name: "<= pass lower", constraint: "<=2.0.0", version: "1.5.0", expected: true},
		{name: "<= fail", constraint: "<=2.0.0", version: "2.1.0", expected: false},
		{name: "> pass", constraint: ">1.0.0", version: "1.0.1", expected: true},
		{name: "> fail equal", constraint: ">1.0.0", version: "1.0.0", expected: false},
		{name: "< pass", constraint: "<2.0.0", version: "1.9.9", expected: true},
		{name: "< fail equal", constraint: "<2.0.0", version: "2.0.0", expected: false},
		{name: "= pass", constraint: "=1.0.0", version: "1.0.0", expected: true},
		{name: "= fail", constraint: "=1.0.0", version: "1.0.1", expected: false},

		// Ranges
		{name: "range pass", constraint: ">=1.0.0 <2.0.0", version: "1.5.0", expected: true},
		{name: "range pass lower bound", constraint: ">=1.0.0 <2.0.0", version: "1.0.0", expected: true},
		{name: "range fail upper bound", constraint: ">=1.0.0 <2.0.0", version: "2.0.0", expected: false},
		{name: "range fail lower", constraint: ">=1.0.0 <2.0.0", version: "0.9.0", expected: false},
		{name: "range fail upper", constraint: ">=1.0.0 <2.0.0", version: "2.1.0", expected: false},

		// Caret constraints
		{name: "caret pass same", constraint: "^1.2.3", version: "1.2.3", expected: true},
		{name: "caret pass minor", constraint: "^1.2.3", version: "1.5.0", expected: true},
		{name: "caret pass patch", constraint: "^1.2.3", version: "1.2.5", expected: true},
		{name: "caret fail major", constraint: "^1.2.3", version: "2.0.0", expected: false},
		{name: "caret fail lower", constraint: "^1.2.3", version: "1.2.2", expected: false},
		{name: "caret 0.x pass", constraint: "^0.2.3", version: "0.2.5", expected: true},
		{name: "caret 0.x fail minor", constraint: "^0.2.3", version: "0.3.0", expected: false},
		{name: "caret 0.0.x pass", constraint: "^0.0.3", version: "0.0.3", expected: true},
		{name: "caret 0.0.x fail patch", constraint: "^0.0.3", version: "0.0.4", expected: false},

		// Tilde constraints
		{name: "tilde pass same", constraint: "~1.2.3", version: "1.2.3", expected: true},
		{name: "tilde pass patch", constraint: "~1.2.3", version: "1.2.5", expected: true},
		{name: "tilde fail minor", constraint: "~1.2.3", version: "1.3.0", expected: false},
		{name: "tilde fail lower", constraint: "~1.2.3", version: "1.2.2", expected: false},

		// Version with v prefix
		{name: "v prefix pass", constraint: ">=1.0.0", version: "v1.5.0", expected: true},
		{name: "v prefix in constraint", constraint: ">=v1.0.0", version: "1.5.0", expected: true},

		// Prerelease versions
		{name: "prerelease pass", constraint: ">=1.0.0-rc1", version: "1.0.0-rc2", expected: true},
		{name: "prerelease release higher", constraint: ">=1.0.0-rc1", version: "1.0.0", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("failed to parse constraint: %v", err)
			}
			v, err := NewVersion(tt.version)
			if err != nil {
				t.Fatalf("failed to parse version: %v", err)
			}
			result := c.Check(v)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
