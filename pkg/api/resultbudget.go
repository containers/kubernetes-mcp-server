package api

import (
	"context"
	"strings"
	"unicode/utf8"
)

// ResultTruncationNotice is appended when a write does not fit in the budget.
// The notice itself may extend the result past the limit by its own length.
const ResultTruncationNotice = "\n[truncated: max_result_bytes]\n"

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

// NewPromptBudget returns the result budget with assistant reserved. The
// assistant text counts toward the same result. A non-positive limit is uncapped.
func NewPromptBudget(ctx context.Context, assistant string) *BuilderWithBudget {
	return NewBuilderWithBudget(ResultLimit(ctx)).Scratch(len(assistant))
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

// Spent reports that another fetch must not start. An exact fill leaves
// Remaining at 0 without Truncated; writing one byte forces the single notice.
func (b *BuilderWithBudget) Spent() bool {
	if b == nil || b.Truncated() {
		return true
	}
	if b.Remaining() == 0 {
		b.WriteString("\n")
		return true
	}
	return false
}

// Fits reports whether n bytes plus at least one more still fit.
// An uncapped builder always fits. A nil or truncated builder does not.
func (b *BuilderWithBudget) Fits(n int) bool {
	if b == nil || b.Truncated() {
		return false
	}
	if b.Remaining() < 0 {
		return true
	}
	if n < 0 {
		n = 0
	}
	return b.Remaining() > int64(n)
}

// Prefix is the text written so far without a trailing truncation notice.
// cut is true when this builder was truncated.
func (b *BuilderWithBudget) Prefix() (text string, cut bool) {
	if b == nil {
		return "", false
	}
	text = b.String()
	if b.Truncated() {
		return strings.TrimSuffix(text, ResultTruncationNotice), true
	}
	return text, false
}

// WriteCut writes text, then one truncation notice when cut.
// An empty text writes nothing. False means the caller should stop fetching.
func (b *BuilderWithBudget) WriteCut(text string, cut bool) bool {
	if b == nil || b.Truncated() {
		return false
	}
	if text == "" {
		return !cut
	}
	if b.WriteString(text) {
		return false
	}
	if cut {
		b.WriteString(ResultTruncationNotice)
	}
	return !b.Truncated()
}

// CommitScratch writes header, the scratch text, and suffix.
// A notice on the scratch is written once, after suffix. An empty scratch
// writes nothing. False means the caller should stop fetching.
func (b *BuilderWithBudget) CommitScratch(header, suffix string, inner *BuilderWithBudget) bool {
	if inner == nil {
		return false
	}
	body, cut := inner.Prefix()
	if strings.TrimSpace(body) == "" {
		return !cut
	}
	return b.WriteCut(header+body+suffix, cut)
}

// Remaining is the bytes this builder can still accept.
// An uncapped builder returns -1. A builder that has already been cut returns 0.
func (b *BuilderWithBudget) Remaining() int64 {
	if b == nil || !b.capped {
		return -1
	}
	if b.truncated {
		return 0
	}
	rem := b.limit - int64(b.b.Len())
	if rem < 0 {
		return 0
	}
	return rem
}

// Scratch returns a builder capped at the bytes this parent can still accept
// after reserve. reserve is the wrapper the parent will add when it commits
// the scratch (a header, a suffix, a trailing newline). An uncapped parent
// returns an uncapped scratch. No room left returns a builder that is already
// truncated, so NewBuilderWithBudget(0) cannot be used to mean "unlimited"
// for a nested collect.
//
// The scratch does not charge the parent. Callers stop fetching when the
// scratch is truncated. See docs/specs/memory-bounds.md#retention.
func (b *BuilderWithBudget) Scratch(reserve int) *BuilderWithBudget {
	if b == nil || !b.capped {
		return NewBuilderWithBudget(0)
	}
	if reserve < 0 {
		reserve = 0
	}
	if b.truncated {
		return &BuilderWithBudget{capped: true, truncated: true}
	}
	room := b.Remaining() - int64(reserve)
	if room <= 0 {
		return &BuilderWithBudget{capped: true, truncated: true}
	}
	return NewBuilderWithBudget(room)
}

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
