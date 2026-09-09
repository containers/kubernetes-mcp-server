package config

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Source names the origin of a resolved option value.
type Source string

const (
	SourceDefault Source = "<Default>"
	SourceEnv     Source = "<Env>"
	SourceTest    Source = "<Test>"
)

const redactedValue = "<redacted>"

// Option is a single configuration value with metadata for each input vector.
// Empty TOMLKey / EnvName means that vector is unavailable.
type Option[T any] struct {
	TOMLKey     string
	EnvName     string
	Description string
	Default     T
	ParseEnv    func(string) (T, error)
	Validate    func(T) error
	Reloadable  bool
	Sensitive   bool
	value       T
	source      Source
}

func (o Option[T]) Get() T         { return o.value }
func (o Option[T]) Source() Source { return o.source }

func (o Option[T]) String() string {
	if o.Sensitive {
		return redactedValue
	}
	return stringify(o.value)
}

// Describe formats the resolved value and its source, e.g. `8080 (<Env>)`.
func (o Option[T]) Describe() string {
	return fmt.Sprintf("%s (%s)", o.String(), o.source)
}

func (o Option[T]) tomlKey() string       { return o.TOMLKey }
func (o Option[T]) envName() string       { return o.EnvName }
func (o Option[T]) reloadable() bool      { return o.Reloadable }
func (o Option[T]) description() string   { return o.Description }
func (o Option[T]) sensitive() bool       { return o.Sensitive }
func (o Option[T]) defaultString() string { return stringify(o.Default) }

// SetForTest sets the value with SourceTest.
func (o *Option[T]) SetForTest(v T) {
	o.setParsed(v, SourceTest)
}

func (o *Option[T]) resetToDefault() {
	o.setParsed(o.Default, SourceDefault)
}

func (o *Option[T]) replaceValue(v T) { o.value = v }

func (o *Option[T]) keepFrom(other option) {
	if src, ok := other.(*Option[T]); ok {
		o.setParsed(src.value, src.source)
	}
}

func (o *Option[T]) equalValue(other option) bool {
	src, ok := other.(*Option[T])
	if !ok {
		return false
	}
	return reflect.DeepEqual(o.value, src.value)
}

func (o *Option[T]) isContainer() bool {
	var zero T
	rt := reflect.TypeOf(zero)
	if rt == nil {
		return false
	}
	if rt.Kind() == reflect.Map {
		return true
	}
	return rt.Kind() == reflect.Slice && rt.Elem().Kind() == reflect.Struct
}

func (o *Option[T]) setParsed(parsed T, src Source) {
	o.value = parsed
	o.source = src
}

func (o *Option[T]) applyTOML(raw any, source Source, path string) error {
	if o.TOMLKey == "" {
		return nil
	}
	parsed, err := parseTyped[T](raw)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if o.isContainer() {
		if err := rejectContainerUnknown(raw, parsed, path); err != nil {
			return err
		}
	}
	o.setParsed(parsed, source)
	return nil
}

func (o *Option[T]) applyEnv() error {
	if o.EnvName == "" {
		return nil
	}
	raw, ok := os.LookupEnv(o.EnvName)
	if !ok || raw == "" {
		return nil
	}
	if o.ParseEnv == nil {
		return fmt.Errorf("option %s is missing ParseEnv", o.EnvName)
	}
	parsed, err := o.ParseEnv(raw)
	if err != nil {
		return fmt.Errorf("env %s: %w", o.EnvName, err)
	}
	o.setParsed(parsed, SourceEnv)
	return nil
}

func (o *Option[T]) validateValue() error {
	if o.Validate == nil {
		return nil
	}
	return o.Validate(o.value)
}

// option is the unexported walk interface so a new Config field cannot be forgotten.
type option interface {
	tomlKey() string
	envName() string
	reloadable() bool
	isContainer() bool
	Describe() string
	Source() Source
	description() string
	sensitive() bool
	defaultString() string
	resetToDefault()
	applyTOML(raw any, source Source, path string) error
	applyEnv() error
	validateValue() error
	equalValue(other option) bool
	keepFrom(other option)
}

func stringify(v any) string {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return "<unset>"
		}
		return fmt.Sprint(rv.Elem().Interface())
	}
	return fmt.Sprint(v)
}

// opt constructs a TOML-backed option. Fluent methods add env, reload, etc.
func opt[T any](key string, def T) Option[T] {
	return Option[T]{
		TOMLKey: key,
		Default: def,
	}
}

func (o Option[T]) reload() Option[T] {
	o.Reloadable = true
	return o
}

func (o Option[T]) env(name string, parse func(string) (T, error)) Option[T] {
	o.EnvName = name
	o.ParseEnv = parse
	return o
}

func (o Option[T]) secret() Option[T] {
	o.Sensitive = true
	return o
}

func (o Option[T]) desc(d string) Option[T] {
	o.Description = d
	return o
}

func (o Option[T]) validate(fn func(T) error) Option[T] {
	o.Validate = fn
	return o
}

func parseTyped[T any](v any) (T, error) {
	var zero T
	switch any(zero).(type) {
	case int:
		n, err := asInt64(v)
		return any(int(n)).(T), err
	case int64:
		n, err := asInt64(v)
		return any(n).(T), err
	case time.Duration:
		s, ok := v.(string)
		if !ok {
			return zero, fmt.Errorf("expected duration string, got %T", v)
		}
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return zero, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return any(parsed).(T), nil
	default:
		return parseTOMLViaDecode[T](v)
	}
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case float64:
		if n != float64(int64(n)) {
			return 0, fmt.Errorf("expected integer, got float %v", n)
		}
		return int64(n), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", v)
	}
}

func parseTOMLViaDecode[T any](v any) (T, error) {
	var zero T
	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(map[string]any{"v": v}); err != nil {
		return zero, err
	}
	var wrap struct {
		V T `toml:"v"`
	}
	if err := toml.Unmarshal(buf.Bytes(), &wrap); err != nil {
		return zero, err
	}
	return wrap.V, nil
}

func parseEnvString(s string) (string, error) { return s, nil }

func parseEnvInt(s string) (int, error) { return strconv.Atoi(s) }

func parseEnvFloat32(s string) (float32, error) {
	n, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, err
	}
	return float32(n), nil
}

func parseEnvFloat64Ptr(s string) (*float64, error) {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func parseEnvStringSlice(s string) ([]string, error) {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

func parseEnvDurationMS(s string) (time.Duration, error) {
	ms, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("expected integer milliseconds, got %q", s)
	}
	if ms < 0 {
		return 0, fmt.Errorf("duration must not be negative (got %d)", ms)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func walkOptions(v any, fn func(o option, path string)) {
	walkValue(reflect.ValueOf(v), "", fn, nil)
}

func walkValue(v reflect.Value, prefix string, fn func(option, string), tables *[]string) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		field := v.Field(i)
		if !field.CanAddr() {
			continue
		}
		if o, ok := field.Addr().Interface().(option); ok {
			if fn == nil {
				continue
			}
			path := joinPath(prefix, o.tomlKey())
			fn(o, path)
			continue
		}
		if field.Kind() == reflect.Struct {
			tag := sf.Tag.Get("toml")
			name, _, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			next := joinPath(prefix, name)
			if name != "" && tables != nil {
				*tables = append(*tables, next)
			}
			walkValue(field, next, fn, tables)
		}
	}
}

func wrappingTables(v any) []string {
	var tables []string
	walkValue(reflect.ValueOf(v), "", nil, &tables)
	return tables
}

func joinPath(prefix, key string) string {
	switch {
	case prefix != "" && key != "":
		return prefix + "." + key
	case prefix != "":
		return prefix
	default:
		return key
	}
}
