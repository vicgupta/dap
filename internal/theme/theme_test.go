package theme

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", System},
		{"  System ", System},
		{"LIGHT", Light},
		{"dark", Dark},
	}
	for _, tc := range cases {
		got, err := Normalize(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("Normalize(%q) = %q, %v", tc.in, got, err)
		}
	}
	if _, err := Normalize("sepia"); err == nil {
		t.Fatal("expected error")
	}
}

func TestStoredRejectsUnknown(t *testing.T) {
	if got := Stored("neon"); got != System {
		t.Fatal(got)
	}
	if got := Stored(" Dark "); got != Dark {
		t.Fatal(got)
	}
}

func TestResolve(t *testing.T) {
	if got := Resolve(System, true); got != Dark {
		t.Fatal(got)
	}
	if got := Resolve(System, false); got != Light {
		t.Fatal(got)
	}
	if got := Resolve(Light, true); got != Light {
		t.Fatal(got)
	}
	if got := Resolve(Dark, false); got != Dark {
		t.Fatal(got)
	}
	if got := Resolve("nope", true); got != Dark {
		t.Fatal(got)
	}
	if got := Resolve("nope", false); got != Light {
		t.Fatal(got)
	}
}
