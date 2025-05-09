package rope

import (
	"testing"
	"unicode/utf8"
)

func TestJSLength(t *testing.T) {
	type testCase struct {
		src      string
		expected int
	}

	cases := []testCase{
		{"", 0},
		{"👍", 2},
		{"𝌆", 2},
		{"hello there", 11},
		{string([]rune{0xd834}), 1},
		{string([]rune{0xdf06}), 1},
		{string([]rune{0xd834, 0xdf06}), 2},
	}

	for _, c := range cases {
		actual := JSLength(c.src)
		t.Logf("s=%v len()=%d runelen()=%d jslen()=%d", c.src, len(c.src), utf8.RuneCountInString(c.src), actual)
		if actual != c.expected {
			t.Errorf("for s=%v actual=%d expected=%d", c.src, actual, c.expected)
		}
	}
}
