package sync

import (
	"testing"
	"time"
)

// Items sharing a date (e.g. a batch rebase) must sort the same way whatever the
// input order, otherwise propagation picks a different "latest" commit each run.
func TestSortTimeline_tieBreakIsDeterministic(t *testing.T) {
	same := time.Date(2026, 9, 28, 11, 57, 28, 0, time.UTC)
	older := same.Add(-time.Hour)
	build := func(order []string) []TimelineItem {
		items := map[string]TimelineItem{
			"c-bbb": NewTimelineComponentsItem("bbb", "bbb", same, nil),
			"c-aaa": NewTimelineComponentsItem("aaa", "aaa", same, nil),
			"c-ccc": NewTimelineComponentsItem("ccc", "ccc", same, nil),
			"v-zzz": NewTimelineVariablesItem("zzz", "zzz", same, nil),
			"v-yyy": NewTimelineVariablesItem("yyy", "yyy", same, nil),
			"c-old": NewTimelineComponentsItem("old", "old", older, nil),
		}
		var list []TimelineItem
		for _, k := range order {
			list = append(list, items[k])
		}
		return list
	}
	versions := func(list []TimelineItem) []string {
		var out []string
		for _, it := range list {
			out = append(out, it.GetVersion())
		}
		return out
	}

	orders := [][]string{
		{"c-bbb", "c-aaa", "c-ccc", "v-zzz", "v-yyy", "c-old"},
		{"c-old", "v-yyy", "c-ccc", "v-zzz", "c-aaa", "c-bbb"},
		{"v-zzz", "c-ccc", "c-old", "c-bbb", "v-yyy", "c-aaa"},
	}
	var want []string
	for n, o := range orders {
		list := build(o)
		SortTimeline(list, SortDesc)
		got := versions(list)
		if n == 0 {
			want = got
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("order %d: got %v, want %v", n, got, want)
			}
		}
	}
	// Newest date first; at equal date, components before variables in desc, each by version.
	expected := []string{"aaa", "bbb", "ccc", "yyy", "zzz", "old"}
	for i := range expected {
		if want[i] != expected[i] {
			t.Fatalf("got %v, want %v", want, expected)
		}
	}
}
