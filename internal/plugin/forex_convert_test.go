package plugin

import "testing"

func TestFxParseConvert(t *testing.T) {
	cases := []struct {
		in        []string
		amt       float64
		base      string
		quote     string
		shouldErr bool
	}{
		{[]string{"1500", "USD", "EUR"}, 1500, "USD", "EUR", false},
		{[]string{"1500", "USD", "to", "EUR"}, 1500, "USD", "EUR", false},
		{[]string{"1500", "USD", "into", "EUR"}, 1500, "USD", "EUR", false},
		{[]string{"1,500.50", "USD", "EUR"}, 1500.50, "USD", "EUR", false},
		{[]string{"1.5k", "USD", "EUR"}, 1500, "USD", "EUR", false},
		{[]string{"1500", "USD/EUR"}, 1500, "USD", "EUR", false},
		{[]string{"USD/EUR", "1500"}, 1500, "USD", "EUR", false},
		{[]string{"1500", "EUR/USD"}, 1500, "EUR", "USD", false},
		{[]string{"1500", "JPY", "USD"}, 1500, "JPY", "USD", false},

		{[]string{"USD", "EUR"}, 0, "", "", true},                 // no amount
		{[]string{"1500", "USD"}, 0, "", "", true},                // missing quote
		{[]string{"1500", "USD", "USD"}, 0, "", "", true},         // same currency
		{[]string{"1500", "ZZZ", "EUR"}, 0, "", "", true},         // unknown currency
		{[]string{"1500", "USD", "EUR", "JPY"}, 0, "", "", true},  // too many currencies
		{[]string{"1500", "100", "USD", "EUR"}, 0, "", "", true},  // multiple amounts
		{[]string{"-100", "USD", "EUR"}, 0, "", "", true},         // negative
	}

	for _, c := range cases {
		amt, base, quote, err := fxParseConvert(c.in)
		if c.shouldErr {
			if err == nil {
				t.Errorf("%v: expected error, got %v %s/%s", c.in, amt, base, quote)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: unexpected error: %v", c.in, err)
			continue
		}
		if amt != c.amt || base != c.base || quote != c.quote {
			t.Errorf("%v: got %v %s/%s, want %v %s/%s", c.in, amt, base, quote, c.amt, c.base, c.quote)
		}
	}
}

func TestFxAddCommas(t *testing.T) {
	cases := []struct {
		v        float64
		decimals int
		want     string
	}{
		{1500, 2, "1,500.00"},
		{1500.5, 2, "1,500.50"},
		{1234567.89, 2, "1,234,567.89"},
		{100, 2, "100.00"},
		{0, 2, "0.00"},
		{232500, 0, "232,500"},
		{-1500, 2, "-1,500.00"},
	}
	for _, c := range cases {
		got := fxAddCommas(c.v, c.decimals)
		if got != c.want {
			t.Errorf("fxAddCommas(%v, %d) = %q, want %q", c.v, c.decimals, got, c.want)
		}
	}
}
