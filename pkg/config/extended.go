package config

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

type ExtendedConfigParser func(ctx context.Context, primitive toml.Primitive, md toml.MetaData) (api.ExtendedConfig, error)

type extendedConfigRegistry struct {
	parsers map[string]ExtendedConfigParser
}

func newExtendedConfigRegistry() *extendedConfigRegistry {
	return &extendedConfigRegistry{
		parsers: make(map[string]ExtendedConfigParser),
	}
}

func (r *extendedConfigRegistry) register(name string, parser ExtendedConfigParser) {
	if _, exists := r.parsers[name]; exists {
		panic("extended config parser already registered for name: " + name)
	}
	r.parsers[name] = parser
}

func (r *extendedConfigRegistry) parseMaps(ctx context.Context, table string, configs map[string]any) (map[string]api.ExtendedConfig, error) {
	if len(configs) == 0 {
		return make(map[string]api.ExtendedConfig), nil
	}
	parsedConfigs := make(map[string]api.ExtendedConfig, len(configs))

	for name, raw := range configs {
		parser, ok := r.parsers[name]
		if !ok {
			return nil, fmt.Errorf("unknown config key %q", table+"."+name)
		}

		primitive, md, err := primitiveFromValue(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to encode extended config for %q: %w", name, err)
		}

		extendedConfig, err := parser(ctx, primitive, md)
		if err != nil {
			return nil, fmt.Errorf("failed to parse extended config for '%s': %w", name, err)
		}

		if child, isMap := raw.(map[string]any); isMap {
			if err := rejectUnknownStructKeys(child, extendedConfig, table+"."+name); err != nil {
				return nil, err
			}
		}

		if err = extendedConfig.Validate(); err != nil {
			return nil, fmt.Errorf("failed to validate extended config for '%s': %w", name, err)
		}

		parsedConfigs[name] = extendedConfig
	}

	return parsedConfigs, nil
}

func primitiveFromValue(v any) (toml.Primitive, toml.MetaData, error) {
	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(map[string]any{"cfg": v}); err != nil {
		return toml.Primitive{}, toml.MetaData{}, err
	}
	var wrap struct {
		Cfg toml.Primitive `toml:"cfg"`
	}
	md, err := toml.NewDecoder(buf).Decode(&wrap)
	if err != nil {
		return toml.Primitive{}, toml.MetaData{}, err
	}
	return wrap.Cfg, md, nil
}

func rejectUnknownStructKeys(raw map[string]any, parsed any, prefix string) error {
	known := tomlFields(parsed)
	for key, val := range raw {
		ft, ok := known[key]
		if !ok {
			return fmt.Errorf("unknown config key %q", prefix+"."+key)
		}
		ft = derefType(ft)
		if child, isMap := val.(map[string]any); isMap && ft.Kind() == reflect.Struct {
			if err := rejectUnknownStructKeys(child, reflect.New(ft).Interface(), prefix+"."+key); err != nil {
				return err
			}
		}
		if ft.Kind() == reflect.Slice && derefType(ft.Elem()).Kind() == reflect.Struct {
			if err := rejectUnknownSlice(val, derefType(ft.Elem()), prefix+"."+key); err != nil {
				return err
			}
		}
		if ft.Kind() == reflect.Map && derefType(ft.Elem()).Kind() == reflect.Struct {
			if child, isMap := val.(map[string]any); isMap {
				if err := rejectUnknownMapOfStruct(child, derefType(ft.Elem()), prefix+"."+key); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func tomlFields(v any) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	rt := reflect.TypeOf(v)
	if rt == nil {
		return out
	}
	if rt.Kind() == reflect.Ptr {
		rt = rt.Elem()
	}
	if rt.Kind() != reflect.Struct {
		return out
	}
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if !sf.IsExported() {
			continue
		}
		tag := sf.Tag.Get("toml")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		out[name] = sf.Type
	}
	return out
}

func derefType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr {
		return t.Elem()
	}
	return t
}

func rejectUnknownSlice(raw any, elem reflect.Type, prefix string) error {
	for i, m := range asSliceOfMaps(raw) {
		if err := rejectUnknownStructKeys(m, reflect.New(elem).Interface(), fmt.Sprintf("%s[%d]", prefix, i)); err != nil {
			return err
		}
	}
	return nil
}

func rejectUnknownMapOfStruct(raw map[string]any, elem reflect.Type, prefix string) error {
	for k, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectUnknownStructKeys(m, reflect.New(elem).Interface(), prefix+"."+k); err != nil {
			return err
		}
	}
	return nil
}

func asSliceOfMaps(raw any) []map[string]any {
	switch s := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(s))
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return s
	default:
		return nil
	}
}

func rejectContainerUnknown(raw, parsed any, path string) error {
	rt := reflect.TypeOf(parsed)
	if rt == nil {
		return nil
	}
	rt = derefType(rt)
	switch {
	case rt.Kind() == reflect.Map && derefType(rt.Elem()).Kind() == reflect.Struct:
		child, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		return rejectUnknownMapOfStruct(child, derefType(rt.Elem()), path)
	case rt.Kind() == reflect.Slice && derefType(rt.Elem()).Kind() == reflect.Struct:
		return rejectUnknownSlice(raw, derefType(rt.Elem()), path)
	case rt.Kind() == reflect.Struct:
		child, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		return rejectUnknownStructKeys(child, parsed, path)
	}
	return nil
}
