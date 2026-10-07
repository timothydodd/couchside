package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLogFileRollsOver(t *testing.T) {
	p := filepath.Join(t.TempDir(), "logs", "couchside.log")
	l, err := openLogFile(p)
	if err != nil {
		t.Fatal(err)
	}
	big := bytes.Repeat([]byte("x"), logFileMax-10)
	if _, err := l.Write(big); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Write([]byte("next line that doesn't fit\n")); err != nil {
		t.Fatal(err)
	}
	l.f.Close()
	if st, err := os.Stat(p + ".1"); err != nil || st.Size() != int64(len(big)) {
		t.Fatalf("old log: %v %v", st, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "next line that doesn't fit\n" {
		t.Fatalf("new log = %q", b)
	}
}
