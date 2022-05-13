package metric

import (
	"testing"
)

func TestValue(t *testing.T) {
	d := NewDesc("group", "", []string{"name"})
	v := NewValue(d, 42, "foobar")

	if v.L("name") != "foobar" {
		t.Fatalf("label name doesn't have the expected value")
	}

	p1 := NewPattern("group")

	if v.Match([]Pattern{p1}) == false {
		t.Fatalf("pattern p1 should have matched")
	}

	p2 := NewPattern("group", "name", "foobar")

	if v.Match([]Pattern{p2}) == false {
		t.Fatalf("pattern p2 should have matched")
	}
}
