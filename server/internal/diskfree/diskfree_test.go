package diskfree

import "testing"

func TestOf(t *testing.T) {
	s, err := Of(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Total == 0 || s.Free > s.Total {
		t.Fatalf("space = %+v", s)
	}
	if _, err := Of("/no/such/folder/anywhere"); err == nil {
		t.Fatal("no error for a missing folder")
	}
}
