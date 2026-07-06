// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package parameters

import (
	"reflect"
	"testing"
)

func sample() *Parameters {
	return New(map[string]any{
		"user": map[string]any{
			"name":  "David",
			"age":   42,
			"admin": true,
			"tags":  []any{"a", "b"},
			"pets": []any{
				map[string]any{"kind": "cat", "secret": "x"},
				map[string]any{"kind": "dog"},
			},
			"prefs":   map[string]any{"theme": "dark", "n": 3},
			"ignored": []any{map[string]any{"z": 1}},
		},
		"token": "abc",
	})
}

func TestNewAndAccessors(t *testing.T) {
	p := New(nil)
	if p.Len() != 0 || p.Permitted() {
		t.Fatalf("empty params bad state")
	}
	p = sample()
	if !p.Has("user") || p.Has("missing") {
		t.Fatalf("Has wrong")
	}
	if got := p.Keys(); !reflect.DeepEqual(got, []string{"token", "user"}) {
		t.Fatalf("Keys not sorted: %v", got)
	}
	// Keys returns a copy.
	p.Keys()[0] = "mut"
	if p.Keys()[0] != "token" {
		t.Fatalf("Keys not copied")
	}
	if _, ok := p.Get("nope"); ok {
		t.Fatalf("Get missing should be false")
	}
	user, _ := p.Get("user")
	if _, ok := user.(*Parameters); !ok {
		t.Fatalf("nested hash not converted to *Parameters")
	}
}

func TestConvertValueParametersPassthrough(t *testing.T) {
	inner := New(map[string]any{"a": "1"})
	p := New(map[string]any{"x": inner})
	got, _ := p.Get("x")
	if got != any(inner) {
		t.Fatalf("existing *Parameters should pass through")
	}
}

func TestEach(t *testing.T) {
	p := New(map[string]any{"a": "1", "b": "2", "c": "3"})
	var seen []string
	p.Each(func(k string, _ any) bool { seen = append(seen, k); return true })
	if !reflect.DeepEqual(seen, []string{"a", "b", "c"}) {
		t.Fatalf("Each order: %v", seen)
	}
	seen = nil
	p.Each(func(k string, _ any) bool { seen = append(seen, k); return false })
	if !reflect.DeepEqual(seen, []string{"a"}) {
		t.Fatalf("Each early stop: %v", seen)
	}
}

func TestRequire(t *testing.T) {
	p := New(map[string]any{
		"a": "x", "empty": "", "nilv": nil, "arr": []any{},
		"hash": map[string]any{}, "false": false,
	})
	if v, err := p.Require("a"); err != nil || v != "x" {
		t.Fatalf("require present: %v %v", v, err)
	}
	if v, err := p.Require("false"); err != nil || v != false {
		t.Fatalf("require false is not blank: %v %v", v, err)
	}
	for _, k := range []string{"empty", "nilv", "arr", "hash", "absent"} {
		if _, err := p.Require(k); err == nil {
			t.Fatalf("require %q should fail", k)
		} else if pm, ok := err.(*ParameterMissing); !ok || pm.Key != k {
			t.Fatalf("wrong error for %q: %v", k, err)
		}
	}
}

func TestRequireAll(t *testing.T) {
	p := New(map[string]any{"a": "1", "b": "2"})
	vs, err := p.RequireAll([]string{"a", "b"})
	if err != nil || !reflect.DeepEqual(vs, []any{"1", "2"}) {
		t.Fatalf("RequireAll ok: %v %v", vs, err)
	}
	if _, err := p.RequireAll([]string{"a", "missing"}); err == nil {
		t.Fatalf("RequireAll should fail on missing")
	}
}

func TestPermitScalarsAndArrays(t *testing.T) {
	p := New(map[string]any{
		"name":   "David",
		"n":      7,
		"arrok":  []any{"1", 2, true, nil},
		"arrbad": []any{"1", map[string]any{"x": 1}},
		"nested": map[string]any{"deep": "v"}, // not requested as scalar
		"nilv":   nil,
	})
	out := p.Permit("name", "n", "nilv", "absentscalar",
		map[string]any{"arrok": []any{}},
		map[string]any{"arrbad": []any{}},
	)
	if !out.Permitted() {
		t.Fatalf("permit output should be permitted")
	}
	if v, _ := out.Get("name"); v != "David" {
		t.Fatalf("scalar name")
	}
	if v, _ := out.Get("n"); v != 7 {
		t.Fatalf("scalar n")
	}
	if !out.Has("nilv") { // NilClass is a permitted scalar in Rails
		t.Fatalf("nil should be a permitted scalar")
	}
	if out.Has("absentscalar") {
		t.Fatalf("absent scalar key must not appear")
	}
	if arr, _ := out.Get("arrok"); !reflect.DeepEqual(arr, []any{"1", 2, true, nil}) {
		t.Fatalf("array of scalars: %v", arr)
	}
	if out.Has("arrbad") {
		t.Fatalf("array with non-scalar must be rejected")
	}
}

func TestPermitNested(t *testing.T) {
	p := sample()
	out := p.Permit("token",
		map[string]any{"user": []any{
			"name", "age", "admin",
			map[string]any{"tags": []any{}},
			map[string]any{"pets": []any{"kind"}},
			map[string]any{"prefs": map[string]any{}},
		}},
	)
	h, err := out.ToH()
	if err != nil {
		t.Fatalf("ToH: %v", err)
	}
	user := h["user"].(map[string]any)
	if user["name"] != "David" || user["age"] != 42 || user["admin"] != true {
		t.Fatalf("nested scalars: %v", user)
	}
	if !reflect.DeepEqual(user["tags"], []any{"a", "b"}) {
		t.Fatalf("nested tags: %v", user["tags"])
	}
	pets := user["pets"].([]any)
	if len(pets) != 2 || pets[0].(map[string]any)["kind"] != "cat" {
		t.Fatalf("nested pets: %v", pets)
	}
	if _, ok := pets[0].(map[string]any)["secret"]; ok {
		t.Fatalf("unpermitted nested key leaked")
	}
	prefs := user["prefs"].(map[string]any)
	if prefs["theme"] != "dark" || prefs["n"] != 3 {
		t.Fatalf("EMPTY_HASH permit-all: %v", prefs)
	}
	if _, ok := user["ignored"]; ok {
		t.Fatalf("ignored key should be absent")
	}
	if _, ok := user["pets"]; !ok {
		t.Fatalf("pets present")
	}
}

func TestPermitEdgeCases(t *testing.T) {
	p := New(map[string]any{
		"scalarAsHash":      "s",                                             // key present, hashFilter but value scalar
		"arrNested":         "notarray",                                      // []any nested filter on scalar -> skip
		"emptyHashOnScalar": "x",                                             // EMPTY_HASH on non-Parameters -> skip
		"nilval":            nil,                                             // v == nil -> continue
		"cfg":               map[string]any{"a": []any{"x", "y"}, "junk": 9}, // non-empty map spec
		"deep": map[string]any{ // EMPTY_HASH permit-all with nested recursion
			"sub":  map[string]any{"x": 1},
			"list": []any{1, 2},
		},
	})
	out := p.Permit(
		map[string]any{"scalarAsHash": []any{"a"}}, // eachElementPermit on scalar -> not allowed
		map[string]any{"arrNested": []any{"a"}},
		map[string]any{"emptyHashOnScalar": map[string]any{}},
		map[string]any{"nilval": []any{}},
		map[string]any{"cfg": map[string]any{"a": []any{}}}, // nested non-empty map spec
		map[string]any{"deep": map[string]any{}},            // permit-all
		map[string]any{"absent": []any{}},                   // key missing -> continue
		map[string]any{"nilspec": nil},                      // spec nil -> continue
		42,                                                  // unknown filter type ignored
	)
	if out.Has("scalarAsHash") || out.Has("arrNested") || out.Has("emptyHashOnScalar") || out.Has("nilval") {
		t.Fatalf("scalar/nil values must not satisfy nested filters: %v", out.Keys())
	}
	cfg, _ := out.Get("cfg")
	cfgp := cfg.(*Parameters)
	if v, _ := cfgp.Get("a"); !reflect.DeepEqual(v, []any{"x", "y"}) || cfgp.Has("junk") {
		t.Fatalf("non-empty map spec not filtered: %v", cfgp)
	}
	deep, _ := out.Get("deep")
	h := deep.(*Parameters).ToUnsafeH()
	if h["sub"].(map[string]any)["x"] != 1 || !reflect.DeepEqual(h["list"], []any{1, 2}) {
		t.Fatalf("deep permit-all recursion: %v", h)
	}
}

func TestPermitArrayOfHashes(t *testing.T) {
	p := New(map[string]any{
		"items": []any{
			map[string]any{"id": 1, "junk": 9},
			"scalar-not-a-hash",
		},
	})
	out := p.Permit(map[string]any{"items": []any{"id"}})
	items, _ := out.Get("items")
	arr := items.([]any)
	if len(arr) != 1 { // the scalar element is dropped
		t.Fatalf("array of hashes filter: %v", arr)
	}
	sub := arr[0].(*Parameters)
	if v, _ := sub.Get("id"); v != 1 || sub.Has("junk") {
		t.Fatalf("nested element not filtered: %v", sub)
	}
}

func TestFlattenFilters(t *testing.T) {
	p := New(map[string]any{"a": "1", "b": "2", "c": "3"})
	out := p.Permit([]any{"a", []string{"b"}}, "c")
	if !out.Has("a") || !out.Has("b") || !out.Has("c") {
		t.Fatalf("flatten filters: %v", out.Keys())
	}
}

func TestToHUnpermitted(t *testing.T) {
	p := New(map[string]any{"a": "1"})
	if _, err := p.ToH(); err == nil {
		t.Fatalf("ToH on unpermitted should error")
	} else if _, ok := err.(*UnfilteredParameters); !ok {
		t.Fatalf("wrong error type: %v", err)
	}
	if h := p.ToUnsafeH(); h["a"] != "1" {
		t.Fatalf("ToUnsafeH: %v", h)
	}
}

func TestToUnsafeHRecursion(t *testing.T) {
	p := sample()
	h := p.ToUnsafeH()
	user := h["user"].(map[string]any)
	if user["tags"].([]any)[0] != "a" {
		t.Fatalf("deep array")
	}
	if user["prefs"].(map[string]any)["theme"] != "dark" {
		t.Fatalf("deep hash")
	}
}

func TestMerge(t *testing.T) {
	a := New(map[string]any{"x": "1", "y": "2"})
	a.permitted = true
	b := New(map[string]any{"y": "9", "z": "3"})
	m := a.Merge(b)
	if v, _ := m.Get("y"); v != "9" {
		t.Fatalf("merge collision other wins")
	}
	if v, _ := m.Get("z"); v != "3" {
		t.Fatalf("merge new key")
	}
	if !m.Permitted() {
		t.Fatalf("merge inherits permitted")
	}
	if m2 := a.Merge(nil); m2.Len() != 2 {
		t.Fatalf("merge nil other")
	}
}

func TestString(t *testing.T) {
	p := New(map[string]any{"a": "1"})
	if p.String() == "" {
		t.Fatalf("String empty")
	}
}

func TestUnpermittedRaise(t *testing.T) {
	defer func() { ActionOnUnpermittedParameters = "" }()
	// Non-raise (log) mode: no panic even with extra keys.
	p := New(map[string]any{"a": "1", "extra": "2"})
	ActionOnUnpermittedParameters = "log"
	_ = p.Permit("a")
	// Raise mode with extra keys: panics.
	ActionOnUnpermittedParameters = "raise"
	func() {
		defer func() {
			r := recover()
			up, ok := r.(*UnpermittedParameters)
			if !ok {
				t.Fatalf("expected *UnpermittedParameters panic, got %v", r)
			}
			if len(up.Keys) != 1 || up.Keys[0] != "extra" {
				t.Fatalf("unpermitted keys: %v", up.Keys)
			}
		}()
		_ = p.Permit("a")
	}()
	// Raise mode, nothing unpermitted: no panic.
	clean := New(map[string]any{"a": "1"})
	_ = clean.Permit("a")
}

func TestIsPermittedScalar(t *testing.T) {
	scalars := []any{
		nil, "s", true,
		int(1), int8(1), int16(1), int32(1), int64(1),
		uint(1), uint8(1), uint16(1), uint32(1), uint64(1),
		float32(1), float64(1),
	}
	for _, v := range scalars {
		if !isPermittedScalar(v) {
			t.Fatalf("%T should be scalar", v)
		}
	}
	for _, v := range []any{[]any{1}, map[string]any{}, New(nil)} {
		if isPermittedScalar(v) {
			t.Fatalf("%T should not be scalar", v)
		}
	}
}

func TestArrayOfPermittedScalars(t *testing.T) {
	if _, ok := arrayOfPermittedScalars("notarray"); ok {
		t.Fatalf("non-array")
	}
	if _, ok := arrayOfPermittedScalars([]any{"a", []any{1}}); ok {
		t.Fatalf("array with non-scalar")
	}
	out, ok := arrayOfPermittedScalars([]any{"a", 1})
	if !ok || len(out) != 2 {
		t.Fatalf("array of scalars: %v %v", out, ok)
	}
}

func TestErrorMessages(t *testing.T) {
	pm := &ParameterMissing{Key: "user"}
	if pm.Error() == "" {
		t.Fatalf("ParameterMissing message")
	}
	one := &UnpermittedParameters{Keys: []string{"a"}}
	if got := one.Error(); got != "found unpermitted parameter: a" {
		t.Fatalf("singular: %q", got)
	}
	many := &UnpermittedParameters{Keys: []string{"b", "a"}}
	if got := many.Error(); got != "found unpermitted parameters: a, b" {
		t.Fatalf("plural+sorted: %q", got)
	}
	if (&UnfilteredParameters{}).Error() == "" {
		t.Fatalf("UnfilteredParameters message")
	}
}
