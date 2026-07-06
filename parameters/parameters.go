// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package parameters is a pure-Go, CGO-free port of
// ActionController::Parameters (Rails "strong parameters"), faithful to
// MRI/Rails 4.0.5-era semantics of require / permit / permitted? / to_h.
//
// A Parameters value wraps an insertion-ordered string-keyed map whose values
// are one of: a scalar (string, bool, integer, float, or nil), a []any array,
// or a nested *Parameters. Construction from a plain Go map converts nested
// maps and array-of-map elements recursively into *Parameters, and orders keys
// deterministically (sorted) since Go maps have no intrinsic order.
package parameters

import (
	"fmt"
	"sort"
)

// ActionOnUnpermittedParameters controls what permit does when the source
// contains keys that were not requested. It mirrors Rails'
// config.action_controller.action_on_unpermitted_parameters:
//
//   - ""      / "log": filter silently (the default).
//   - "raise":         panic with *UnpermittedParameters (Rails raises an
//     exception, which the controller layer turns into a rescuable error).
var ActionOnUnpermittedParameters = ""

// Parameters is the Go analogue of ActionController::Parameters.
type Parameters struct {
	keys      []string
	values    map[string]any
	permitted bool
}

// New builds a Parameters from a plain Go map (which may be nil for an empty
// set). Nested map[string]any values become nested *Parameters, and array
// elements that are maps are converted as well. The resulting Parameters is not
// permitted.
func New(src map[string]any) *Parameters {
	p := &Parameters{values: map[string]any{}}
	keys := make([]string, 0, len(src))
	for k := range src {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p.set(k, convertValue(src[k]))
	}
	return p
}

// convertValue normalises an arbitrary Go value into the Parameters value
// model, recursing into maps and slices.
func convertValue(v any) any {
	switch tv := v.(type) {
	case map[string]any:
		return New(tv)
	case *Parameters:
		return tv
	case []any:
		out := make([]any, len(tv))
		for i, el := range tv {
			out[i] = convertValue(el)
		}
		return out
	default:
		return v
	}
}

// set stores a value, appending the key to the order when new.
func (p *Parameters) set(key string, val any) {
	if _, ok := p.values[key]; !ok {
		p.keys = append(p.keys, key)
	}
	p.values[key] = val
}

// Keys returns the keys in order. The slice is a copy.
func (p *Parameters) Keys() []string {
	out := make([]string, len(p.keys))
	copy(out, p.keys)
	return out
}

// Len reports the number of top-level keys.
func (p *Parameters) Len() int { return len(p.keys) }

// Has reports whether key is present.
func (p *Parameters) Has(key string) bool {
	_, ok := p.values[key]
	return ok
}

// Get returns the value for key and whether it was present. A nested hash is
// returned as *Parameters, matching Rails' [] accessor.
func (p *Parameters) Get(key string) (any, bool) {
	v, ok := p.values[key]
	return v, ok
}

// Permitted reports whether the parameters have been marked permitted.
func (p *Parameters) Permitted() bool { return p.permitted }

// Each iterates key/value pairs in order. Returning false from fn stops.
func (p *Parameters) Each(fn func(key string, val any) bool) {
	for _, k := range p.keys {
		if !fn(k, p.values[k]) {
			return
		}
	}
}

// Require returns the value for key, raising ParameterMissing (as an error)
// when the key is absent or its value is "blank" (nil, "", empty array, or
// empty hash). The literal value false is not blank, matching Rails.
func (p *Parameters) Require(key string) (any, error) {
	v, ok := p.Get(key)
	if ok && !isBlank(v) {
		return v, nil
	}
	return nil, &ParameterMissing{Key: key, Params: p.Keys()}
}

// RequireAll requires each key in turn, returning the values in order. The
// first missing key produces the error, mirroring params.require([:a, :b]).
func (p *Parameters) RequireAll(keys []string) ([]any, error) {
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		v, err := p.Require(k)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// isBlank reports Rails' blank? for the Parameters value model.
func isBlank(v any) bool {
	switch tv := v.(type) {
	case nil:
		return true
	case string:
		return tv == ""
	case []any:
		return len(tv) == 0
	case *Parameters:
		return tv.Len() == 0
	default:
		return false
	}
}

// Permit returns a new, permitted Parameters containing only the values
// allowed by filters. Each filter is either:
//
//   - a string: permit a scalar under that key;
//   - a map[string]any{key: []any{}}: permit an array of scalars under key;
//   - a map[string]any{key: []any{"a", "b", ...}}: permit a nested hash (or
//     array of hashes) whose own keys are filtered by the inner filters;
//   - a map[string]any{key: map[string]any{}}: permit an arbitrary nested hash
//     wholesale (Rails' `key: {}`).
//
// Slice filters are flattened, so Permit("a", "b", []any{"c"}) works. When a
// source key is present but not requested and ActionOnUnpermittedParameters is
// "raise", Permit panics with *UnpermittedParameters.
func (p *Parameters) Permit(filters ...any) *Parameters {
	out := &Parameters{values: map[string]any{}}
	for _, f := range flattenFilters(filters) {
		switch fv := f.(type) {
		case string:
			p.permittedScalarFilter(out, fv)
		case map[string]any:
			p.hashFilter(out, fv)
		}
	}
	p.checkUnpermitted(out)
	out.permitted = true
	return out
}

// flattenFilters expands nested []any / []string filter lists into a flat list.
func flattenFilters(filters []any) []any {
	out := make([]any, 0, len(filters))
	for _, f := range filters {
		switch fv := f.(type) {
		case []any:
			out = append(out, flattenFilters(fv)...)
		case []string:
			for _, s := range fv {
				out = append(out, s)
			}
		default:
			out = append(out, f)
		}
	}
	return out
}

func (p *Parameters) permittedScalarFilter(out *Parameters, key string) {
	if v, ok := p.Get(key); ok && isPermittedScalar(v) {
		out.set(key, v)
	}
}

func (p *Parameters) hashFilter(out *Parameters, filter map[string]any) {
	for key, spec := range filter {
		v, ok := p.Get(key)
		if !ok || v == nil || spec == nil {
			continue
		}
		switch s := spec.(type) {
		case []any:
			if len(s) == 0 {
				if arr, allowed := arrayOfPermittedScalars(v); allowed {
					out.set(key, arr)
				}
				continue
			}
			if res, allowed := eachElementPermit(v, s); allowed {
				out.set(key, res)
			}
		case map[string]any:
			if len(s) == 0 {
				if sub, isParams := v.(*Parameters); isParams {
					out.set(key, sub.deepPermit())
				}
				continue
			}
			if res, allowed := eachElementPermit(v, []any{s}); allowed {
				out.set(key, res)
			}
		}
	}
}

// eachElementPermit permits a nested hash or an array of nested hashes against
// filters. It reports allowed=false when v is neither.
func eachElementPermit(v any, filters []any) (any, bool) {
	switch tv := v.(type) {
	case *Parameters:
		return tv.Permit(filters...), true
	case []any:
		res := make([]any, 0, len(tv))
		for _, el := range tv {
			if sub, ok := el.(*Parameters); ok {
				res = append(res, sub.Permit(filters...))
			}
		}
		return res, true
	default:
		return nil, false
	}
}

// checkUnpermitted implements the action_on_unpermitted_parameters policy.
func (p *Parameters) checkUnpermitted(out *Parameters) {
	if ActionOnUnpermittedParameters != "raise" {
		return
	}
	var unpermitted []string
	for _, k := range p.keys {
		if !out.Has(k) {
			unpermitted = append(unpermitted, k)
		}
	}
	if len(unpermitted) > 0 {
		panic(&UnpermittedParameters{Keys: unpermitted})
	}
}

// deepPermit returns a permitted deep copy of p (used by the `key: {}` filter).
func (p *Parameters) deepPermit() *Parameters {
	out := &Parameters{values: map[string]any{}, permitted: true}
	for _, k := range p.keys {
		out.set(k, deepPermitValue(p.values[k]))
	}
	return out
}

func deepPermitValue(v any) any {
	switch tv := v.(type) {
	case *Parameters:
		return tv.deepPermit()
	case []any:
		out := make([]any, len(tv))
		for i, el := range tv {
			out[i] = deepPermitValue(el)
		}
		return out
	default:
		return v
	}
}

// ToH returns a deep plain-map snapshot of the permitted parameters. It returns
// *UnfilteredParameters when the parameters have not been permitted, mirroring
// ActionController::Parameters#to_h.
func (p *Parameters) ToH() (map[string]any, error) {
	if !p.permitted {
		return nil, &UnfilteredParameters{}
	}
	return p.ToUnsafeH(), nil
}

// ToUnsafeH returns a deep plain-map snapshot regardless of the permitted flag,
// mirroring #to_unsafe_h.
func (p *Parameters) ToUnsafeH() map[string]any {
	out := make(map[string]any, len(p.keys))
	for _, k := range p.keys {
		out[k] = toPlainValue(p.values[k])
	}
	return out
}

func toPlainValue(v any) any {
	switch tv := v.(type) {
	case *Parameters:
		return tv.ToUnsafeH()
	case []any:
		out := make([]any, len(tv))
		for i, el := range tv {
			out[i] = toPlainValue(el)
		}
		return out
	default:
		return v
	}
}

// Merge returns a new Parameters with p's pairs overlaid by other's (other wins
// on collision). The result inherits p's permitted flag.
func (p *Parameters) Merge(other *Parameters) *Parameters {
	out := &Parameters{values: map[string]any{}, permitted: p.permitted}
	for _, k := range p.keys {
		out.set(k, p.values[k])
	}
	if other != nil {
		for _, k := range other.keys {
			out.set(k, other.values[k])
		}
	}
	return out
}

// String renders a stable debug form.
func (p *Parameters) String() string {
	return fmt.Sprintf("<Parameters %v permitted=%t>", p.ToUnsafeH(), p.permitted)
}

// isPermittedScalar reports whether v is a Rails "permitted scalar": a string,
// bool, nil, or any integer/float numeric type. Arrays and hashes are not.
func isPermittedScalar(v any) bool {
	switch v.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

// arrayOfPermittedScalars reports whether v is a []any whose elements are all
// permitted scalars, returning a copy when so.
func arrayOfPermittedScalars(v any) ([]any, bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	for _, el := range arr {
		if !isPermittedScalar(el) {
			return nil, false
		}
	}
	out := make([]any, len(arr))
	copy(out, arr)
	return out, true
}
