// Package logbuf keeps the server's recent log lines in memory for the
// System → Console page, alongside the normal stdout log.
package logbuf

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Entry is one log line.
type Entry struct {
	Seq   int64  `json:"seq"`
	Time  int64  `json:"t"` // unix milliseconds
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Attrs string `json:"attrs"` // key=value pairs, as the text log writes them
}

// Buffer is a ring of the last Size entries.
type Buffer struct {
	mu      sync.Mutex
	size    int
	entries []Entry
	seq     int64
}

func New(size int) *Buffer { return &Buffer{size: size} }

func (b *Buffer) add(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	e.Seq = b.seq
	if len(b.entries) >= b.size {
		// Drop the oldest quarter at once rather than shifting on every line.
		b.entries = append(b.entries[:0], b.entries[len(b.entries)-b.size*3/4:]...)
	}
	b.entries = append(b.entries, e)
}

// Since returns the entries after seq (0 for all that are kept), and the
// last seq to pass next time. gap is true when lines after seq were dropped.
func (b *Buffer) Since(seq int64) (out []Entry, last int64, gap bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out = []Entry{}
	if seq > b.seq {
		seq, gap = 0, true // the server restarted since the caller's last look
	}
	if len(b.entries) == 0 {
		return out, b.seq, false
	}
	first := b.entries[0].Seq
	gap = gap || seq > 0 && seq+1 < first
	i := 0
	if seq >= first {
		i = int(seq - first + 1)
	}
	if i < len(b.entries) {
		out = append(out, b.entries[i:]...)
	}
	return out, b.seq, gap
}

// Handler sends each record to next and keeps a copy in the buffer.
func (b *Buffer) Handler(next slog.Handler) slog.Handler {
	return &handler{buf: b, next: next}
}

type handler struct {
	buf    *Buffer
	next   slog.Handler
	prefix string // attrs from WithAttrs, already formatted
	group  string // "a.b." from WithGroup
}

func (h *handler) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(h.prefix)
	r.Attrs(func(a slog.Attr) bool {
		appendAttr(&sb, h.group, a)
		return true
	})
	t := r.Time
	if t.IsZero() {
		t = time.Now()
	}
	h.buf.add(Entry{Time: t.UnixMilli(), Level: r.Level.String(), Msg: r.Message, Attrs: strings.TrimSpace(sb.String())})
	return h.next.Handle(ctx, r)
}

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	var sb strings.Builder
	sb.WriteString(h.prefix)
	for _, a := range as {
		appendAttr(&sb, h.group, a)
	}
	return &handler{buf: h.buf, next: h.next.WithAttrs(as), prefix: sb.String(), group: h.group}
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &handler{buf: h.buf, next: h.next.WithGroup(name), prefix: h.prefix, group: h.group + name + "."}
}

func appendAttr(sb *strings.Builder, group string, a slog.Attr) {
	v := a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if v.Kind() == slog.KindGroup {
		g := group
		if a.Key != "" {
			g += a.Key + "."
		}
		for _, ga := range v.Group() {
			appendAttr(sb, g, ga)
		}
		return
	}
	sb.WriteByte(' ')
	sb.WriteString(group)
	sb.WriteString(a.Key)
	sb.WriteByte('=')
	sb.WriteString(quote(v.String()))
}

// quote quotes values the way the text handler does: when they're empty or
// hold spaces, quotes, '=' or control characters.
func quote(s string) string {
	if s == "" {
		return `""`
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r == '"' || r == '=' || !unicode.IsPrint(r) {
			return strconv.Quote(s)
		}
	}
	return s
}
