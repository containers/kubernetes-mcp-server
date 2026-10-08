package api

import (
	"context"
	"strings"
	"unicode/utf8"
)

// ResultTruncationNotice is appended when a write does not fit in the budget.
// The notice itself may extend the result past the limit by its own length.
const ResultTruncationNotice = "\n[truncated: mcp_max_result_bytes]\n"

type resultLimitKey struct{}

// WithResultBudget records limit on ctx. limit <= 0 disables the cap and
// returns ctx unchanged. A positive limit with a nil ctx uses context.Background.
func WithResultBudget(ctx context.Context, limit int64) context.Context {
	if limit <= 0 {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, resultLimitKey{}, limit)
}

// ResultLimit returns the cap stored by WithResultBudget, or 0 when there is none.
func ResultLimit(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	limit, _ := ctx.Value(resultLimitKey{}).(int64)
	return limit
}

// BuilderWithBudget writes strings up to limit. A write that does not fit is
// cut to the bytes still available, then ResultTruncationNotice is appended.
// Further writes are ignored. limit <= 0 writes every byte.
type BuilderWithBudget struct {
	b         strings.Builder
	limit     int64
	capped    bool
	truncated bool
}

// NewBuilderWithBudget returns a builder capped at limit. limit <= 0 is uncapped.
func NewBuilderWithBudget(limit int64) *BuilderWithBudget {
	if limit <= 0 {
		return &BuilderWithBudget{}
	}
	return &BuilderWithBudget{limit: limit, capped: true}
}

// WriteString appends s, or a prefix of s plus the truncation notice when s
// does not fit. truncated is true when this or an earlier write was cut.
func (b *BuilderWithBudget) WriteString(s string) (truncated bool) {
	if b.truncated {
		return true
	}
	if !b.capped {
		b.b.WriteString(s)
		return false
	}
	remainder := b.limit - int64(b.b.Len())
	if remainder < 0 {
		remainder = 0
	}
	if remainder >= int64(len(s)) {
		b.b.WriteString(s)
		return false
	}
	end := int(remainder)
	for end > 0 && end < len(s) && !utf8.RuneStart(s[end]) {
		end--
	}
	if end > 0 {
		b.b.WriteString(s[:end])
	}
	b.b.WriteString(ResultTruncationNotice)
	b.truncated = true
	return true
}

// String returns the bytes written so far, including a truncation notice.
func (b *BuilderWithBudget) String() string { return b.b.String() }

// Len is the number of bytes written.
func (b *BuilderWithBudget) Len() int { return b.b.Len() }

// Truncated reports whether a write has already been cut.
func (b *BuilderWithBudget) Truncated() bool { return b.truncated }

// LimitString writes s under the limit stored on ctx.
func LimitString(ctx context.Context, s string) string {
	b := NewBuilderWithBudget(ResultLimit(ctx))
	b.WriteString(s)
	return b.String()
}

// FitSections cuts texts in order so they fit in room. room <= 0 leaves no
// payload bytes: the first non-empty text is replaced by the truncation notice
// and the rest are empty. An uncapped caller should not use this.
func FitSections(room int64, texts ...string) []string {
	out := make([]string, len(texts))
	if room < 0 {
		room = 0
	}
	for i, text := range texts {
		if text == "" {
			continue
		}
		if room <= 0 {
			out[i] = ResultTruncationNotice
			return out
		}
		b := NewBuilderWithBudget(room)
		if b.WriteString(text) {
			out[i] = b.String()
			return out
		}
		out[i] = b.String()
		room -= int64(len(out[i]))
	}
	return out
}
