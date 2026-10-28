package httpui

import "testing"

func TestCSVTextEscapesSpreadsheetFormulas(t *testing.T) {
	for _, value := range []string{"=1+1", "+cmd", "-42", "@SUM(A1)", "  =1", "\tplain", "\rplain", "\nplain", "\u2003=1"} {
		if got := csvText(value); got != "'"+value {
			t.Fatalf("unsafe CSV %q: %q", value, got)
		}
	}
	for _, value := range []string{"", "ordinary text", "name@example.com", "2+2", "  plain"} {
		if got := csvText(value); got != value {
			t.Fatalf("changed safe CSV %q", value)
		}
	}
}
