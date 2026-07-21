package plugin

import "testing"

// TestRobbieGiftCount pins the two tracks: the every-10th-visit loyalty gift
// and the volume track that pays for a big haul, capped so one monster
// stockpile can't mint an unbounded pile of consumables.
func TestRobbieGiftCount(t *testing.T) {
	cases := []struct {
		name          string
		visits, taken int
		want          int
	}{
		{"small haul, off-loyalty visit", 7, 3, 0},
		{"loyalty visit only", 10, 3, 1},
		{"haul only", 7, 15, 1},
		{"haul and loyalty stack", 20, 15, 2},
		{"haul scales", 7, 45, 3},
		{"haul capped", 7, 500, robbieMaxHaulGifts},
		{"cap plus loyalty", 30, 500, robbieMaxHaulGifts + 1},
		{"one under the haul threshold", 7, robbieHaulPerGift - 1, 0},
		{"zeroth visit is not a loyalty visit", 0, 0, 0},
	}
	for _, c := range cases {
		if got := robbieGiftCount(c.visits, c.taken); got != c.want {
			t.Errorf("%s: robbieGiftCount(%d, %d) = %d, want %d",
				c.name, c.visits, c.taken, got, c.want)
		}
	}
}

func TestJoinAnd(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b and c"},
	}
	for _, c := range cases {
		if got := joinAnd(c.in); got != c.want {
			t.Errorf("joinAnd(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
