package scheduler

import (
	"testing"
	"time"
)

func TestEveryNext(t *testing.T) {
	from := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	next, err := Next(Spec{Every: "15m"}, from)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Equal(from.Add(15 * time.Minute)) {
		t.Fatalf("next = %s", next)
	}
}

func TestCronMondayMorning(t *testing.T) {
	// Sunday 1 Nov 2026, 12:00 UTC. Next Monday 08:00.
	from := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	next, err := Next(Spec{Cron: "0 8 * * 1"}, from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s want %s", next, want)
	}
}

func TestWeekdaysSkipWeekend(t *testing.T) {
	loc := time.UTC
	// Friday 6 Nov 2026 09:00 -> Monday 9 Nov 08:00.
	from := time.Date(2026, 11, 6, 9, 0, 0, 0, loc)
	next, err := Next(Spec{At: "08:00", Recurrence: "weekdays"}, from)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 9, 8, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("next = %s want %s", next, want)
	}
	// Sunday noon -> Monday 08:00.
	sun := time.Date(2026, 11, 1, 12, 0, 0, 0, loc)
	next, err = Next(Spec{At: "08:00", Recurrence: "weekdays"}, sun)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, 11, 2, 8, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("sunday next = %s", next)
	}
}

func TestWeeklyDay(t *testing.T) {
	from := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) // Sunday
	next, err := Next(Spec{At: "08:00", Recurrence: "weekly", Day: "wed"}, from)
	if err != nil {
		t.Fatal(err)
	}
	if next.Weekday() != time.Wednesday || next.Hour() != 8 {
		t.Fatalf("next = %s", next)
	}
}

func TestOnTimeWithinGrace(t *testing.T) {
	spec := Spec{Every: "30m"}
	due := time.Date(2026, 11, 1, 7, 0, 0, 0, time.UTC)
	now := due.Add(30 * time.Second)
	d, err := Evaluate(spec, State{NextRun: due}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Fire || d.Trigger != "schedule" || d.Missed != 0 {
		t.Fatalf("decision = %+v", d)
	}
}

func TestMissedCatchUpOnce(t *testing.T) {
	spec := Spec{Every: "30m"}
	due := time.Date(2026, 11, 1, 7, 0, 0, 0, time.UTC)
	// 7:00, 7:30, 8:00, 8:30 are due; now is 8:40. Four missed, one catch-up.
	now := time.Date(2026, 11, 1, 8, 40, 0, 0, time.UTC)
	d, err := Evaluate(spec, State{NextRun: due}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Fire || d.Trigger != "catchup" {
		t.Fatalf("decision = %+v", d)
	}
	if d.Missed != 4 {
		t.Fatalf("missed = %d", d.Missed)
	}
	if d.Reason == "" {
		t.Fatal("missing reason")
	}
	if !d.Next.After(now) {
		t.Fatalf("next %s should be after now", d.Next)
	}
	// The decision fires once. Replaying Evaluate with the advanced cursor does not
	// ask for the skipped slots again.
	again, err := Evaluate(spec, State{NextRun: d.Next}, now)
	if err != nil {
		t.Fatal(err)
	}
	if again.Fire {
		t.Fatal("catch-up must not replay missed slots")
	}
}

func TestNotDue(t *testing.T) {
	spec := Spec{Every: "1h"}
	now := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	d, err := Evaluate(spec, State{NextRun: now.Add(time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if d.Fire {
		t.Fatal("should not fire")
	}
}

func TestBadCron(t *testing.T) {
	if err := Validate(Spec{Cron: "not a cron"}); err == nil {
		t.Fatal("expected error")
	}
}
