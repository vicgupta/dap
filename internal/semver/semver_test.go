package semver

import "testing"

func TestParse(t *testing.T) {
	v, err := Parse("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1.2.3" {
		t.Fatalf("got %s", v)
	}
	v, err = Parse("0.3")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "0.3.0" {
		t.Fatalf("got %s", v)
	}
	if _, err := Parse("1.2.3-beta"); err == nil {
		t.Fatal("expected pre-release to be rejected")
	}
	if _, err := Parse("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSatisfies(t *testing.T) {
	cases := []struct {
		version, constraint string
		ok                  bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.4", "1.2.3", false},
		{"1.2.3", "=1.2.3", true},
		{"1.2.3", ">=1.2.0", true},
		{"1.1.9", ">=1.2.0", false},
		{"2.0.0", "^1.2.3", false},
		{"1.9.0", "^1.2.3", true},
		{"1.2.2", "^1.2.3", false},
		{"0.2.9", "^0.2.3", true},
		{"0.3.0", "^0.2.3", false},
		{"0.0.3", "^0.0.3", true},
		{"0.0.4", "^0.0.3", false},
		{"1.2.9", "~1.2.3", true},
		{"1.3.0", "~1.2.3", false},
		{"0.3.1", "^0.3", true},
		{"0.4.0", "^0.3", false},
		{"1.0.0", ">=1.0.0 <2.0.0", true},
		{"2.0.0", ">=1.0.0 <2.0.0", false},
		{"9.9.9", "*", true},
		{"0.1.0", "", true},
	}
	for _, tc := range cases {
		ok, err := Satisfies(tc.version, tc.constraint)
		if err != nil {
			t.Fatalf("%s vs %s: %v", tc.version, tc.constraint, err)
		}
		if ok != tc.ok {
			t.Fatalf("%s vs %q: got %v want %v", tc.version, tc.constraint, ok, tc.ok)
		}
	}
}

func TestBadRange(t *testing.T) {
	if err := ValidRange(">>1.0"); err == nil {
		t.Fatal("expected bad range")
	}
	if err := ValidRange("^0.3"); err != nil {
		t.Fatal(err)
	}
}
