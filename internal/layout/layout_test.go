package layout

import "testing"

func TestNormalizeDefaultsAndRejects(t *testing.T) {
	got, err := Normalize(Settings{})
	if err != nil {
		t.Fatal(err)
	}
	def := Default()
	if got.Home.View != def.Home.View || got.Home.Density != def.Home.Density || got.Inbox.View != def.Inbox.View || got.Inbox.Density != def.Inbox.Density {
		t.Fatalf("defaults %+v", got)
	}
	if len(got.Home.Order) != 0 || len(got.Inbox.Pinned) != 0 {
		t.Fatalf("defaults should omit lists: %+v", got)
	}
	if _, err := Normalize(Settings{Home: View{View: "split"}}); err == nil {
		t.Fatal("home split should fail")
	}
	if _, err := Normalize(Settings{Inbox: View{View: "kanban"}}); err == nil {
		t.Fatal("unknown inbox view should fail")
	}
	if _, err := Normalize(Settings{Home: View{View: Grid, Density: "roomy"}}); err == nil {
		t.Fatal("unknown density should fail")
	}
	ok, err := Normalize(Settings{
		Home:  View{View: " Board ", Density: "COMPACT", Pinned: []string{"b", "b", " a "}, Sizes: map[string]string{"hello-watch": " LARGE "}},
		Inbox: View{View: "split"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok.Home.View != Board || ok.Home.Density != Compact || ok.Inbox.View != Split || ok.Inbox.Density != Comfortable {
		t.Fatalf("%+v", ok)
	}
	if len(ok.Home.Pinned) != 2 || ok.Home.Pinned[0] != "b" || ok.Home.Pinned[1] != "a" {
		t.Fatalf("pinned %#v", ok.Home.Pinned)
	}
	if ok.Home.Sizes["hello-watch"] != Large {
		t.Fatal(ok.Home.Sizes)
	}
	if ok.PluginDensity() != Compact {
		t.Fatal(ok.PluginDensity())
	}
}

func TestStoredCoerces(t *testing.T) {
	got := Stored(Settings{Home: View{View: "nope", Density: "nope", Sizes: map[string]string{"x": "huge"}}, Inbox: View{View: "grid"}})
	if got.Home.View != Grid || got.Home.Density != Comfortable || got.Inbox.View != Grid {
		t.Fatalf("%+v", got)
	}
	if len(got.Home.Sizes) != 0 {
		t.Fatal(got.Home.Sizes)
	}
}

func TestResolveSize(t *testing.T) {
	if got := ResolveSize("", "large", "medium"); got != Large {
		t.Fatal(got)
	}
	if got := ResolveSize("small", "large", "medium"); got != Medium {
		t.Fatal(got)
	}
	if got := ResolveSize("large", "small", "small"); got != Large {
		t.Fatal(got)
	}
	if got := ResolveSize("", "", ""); got != Medium {
		t.Fatal(got)
	}
	if ChosenDensity("nope", "compact") != Compact {
		t.Fatal("fallback")
	}
	if ChosenDensity("comfortable", "compact") != Comfortable {
		t.Fatal("query wins")
	}
}
