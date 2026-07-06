// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package dispatch

import (
	"errors"
	"reflect"
	"testing"

	"github.com/go-ruby-rack/rack"
)

// errInput is a rack body seam whose Read always fails.
type errInput struct{}

func (errInput) Read(int) ([]byte, error) { return nil, errors.New("boom") }

// stringInput is a rack body seam yielding a fixed string once.
type stringInput struct {
	s    string
	done bool
}

func (in *stringInput) Read(int) ([]byte, error) {
	if in.done {
		return nil, nil
	}
	in.done = true
	return []byte(in.s), nil
}

func TestPathParametersRoundTrip(t *testing.T) {
	r := NewRequest(rack.Env{})
	r.SetPathParameters(map[string]any{"controller": "posts", "action": "show", "id": "5"})
	if r.ControllerName() != "posts" || r.ActionName() != "show" {
		t.Fatalf("controller/action: %q %q", r.ControllerName(), r.ActionName())
	}
	pp := r.PathParameters()
	if pp["id"] != "5" {
		t.Fatalf("path params: %v", pp)
	}
	// PathParameters returns a copy.
	pp["id"] = "mut"
	if r.PathParameters()["id"] != "5" {
		t.Fatalf("PathParameters must copy")
	}
	// SetPathParameters copies its input.
	src := map[string]any{"x": "1"}
	r.SetPathParameters(src)
	src["x"] = "mut"
	if r.PathParameters()["x"] != "1" {
		t.Fatalf("SetPathParameters must copy")
	}
	// Absent keys yield "".
	empty := NewRequest(rack.Env{})
	if empty.ControllerName() != "" || empty.ActionName() != "" {
		t.Fatalf("absent params should be empty")
	}
}

func TestQueryAndRequestParameters(t *testing.T) {
	r := NewRequest(rack.Env{
		rack.RequestMethod: "GET",
		rack.QueryString:   "a=1&b=2",
	})
	q, err := r.QueryParameters()
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if q["a"] != "1" || q["b"] != "2" {
		t.Fatalf("query params: %v", q)
	}
	if rp, err := r.RequestParameters(); err != nil || len(rp) != 0 {
		t.Fatalf("empty body params: %v %v", rp, err)
	}
}

func TestParametersMerge(t *testing.T) {
	r := NewRequest(rack.Env{
		rack.RequestMethod: "GET",
		rack.QueryString:   "a=q&shared=fromquery",
	})
	r.SetPathParameters(map[string]any{"controller": "c", "id": "9"})
	p, err := r.Parameters()
	if err != nil {
		t.Fatalf("parameters: %v", err)
	}
	if v, _ := p.Get("a"); v != "q" {
		t.Fatalf("query merged: %v", v)
	}
	if v, _ := p.Get("id"); v != "9" {
		t.Fatalf("path merged: %v", v)
	}
	if v, _ := p.Get("controller"); v != "c" {
		t.Fatalf("controller merged: %v", v)
	}
}

func TestParametersNested(t *testing.T) {
	// The rack port nests only body params (via ParseNestedQuery); a POST form
	// body exercises the nested-map and array conversion in rackValueToPlain.
	r := NewRequest(rack.Env{
		rack.RequestMethod: "POST",
		rack.RackInput:     &stringInput{s: "user[name]=Dave&tags[]=a&tags[]=b"},
	})
	p, err := r.Parameters()
	if err != nil {
		t.Fatalf("parameters: %v", err)
	}
	h := p.ToUnsafeH()
	if h["user"].(map[string]any)["name"] != "Dave" {
		t.Fatalf("nested hash: %v", h["user"])
	}
	if !reflect.DeepEqual(h["tags"], []any{"a", "b"}) {
		t.Fatalf("nested array: %v", h["tags"])
	}
}

func TestQueryParameterError(t *testing.T) {
	r := NewRequest(rack.Env{
		rack.RequestMethod: "GET",
		rack.QueryString:   "%zz", // invalid percent-encoding
	})
	if _, err := r.QueryParameters(); err == nil {
		t.Fatalf("expected query parse error")
	}
	if _, err := r.Parameters(); err == nil {
		t.Fatalf("Parameters should surface query error")
	}
}

func TestRequestParameterError(t *testing.T) {
	env := rack.Env{
		rack.RequestMethod: "POST", // no content type -> form data
		rack.RackInput:     errInput{},
	}
	r := NewRequest(env)
	if _, err := r.RequestParameters(); err == nil {
		t.Fatalf("expected body read error")
	}
	// Parameters surfaces the body error after a successful (empty) query.
	r2 := NewRequest(rack.Env{
		rack.RequestMethod: "POST",
		rack.RackInput:     errInput{},
	})
	if _, err := r2.Parameters(); err == nil {
		t.Fatalf("Parameters should surface body error")
	}
}

func TestFormat(t *testing.T) {
	r := NewRequest(rack.Env{})
	r.SetPathParameters(map[string]any{"format": "xml"})
	if r.Format() != "xml" {
		t.Fatalf("path format wins: %s", r.Format())
	}
	cases := map[string]string{
		"application/json":            "json",
		"text/json":                   "json",
		"application/xml":             "xml",
		"text/xml":                    "xml",
		"text/html":                   "html",
		"application/xhtml+xml":       "html",
		"text/html; charset=utf-8":    "html",
		"application/json, text/html": "json",
		"application/octet-stream":    "html", // unrecognised -> default
		"":                            "html",
	}
	for accept, want := range cases {
		r := NewRequest(rack.Env{"HTTP_ACCEPT": accept})
		if got := r.Format(); got != want {
			t.Errorf("Accept %q -> %q, want %q", accept, got, want)
		}
	}
}

func TestStringParamNonString(t *testing.T) {
	if stringParam(map[string]any{"k": 42}, "k") != "" {
		t.Fatalf("non-string param should yield empty string")
	}
}

func TestRackParamsToMapNil(t *testing.T) {
	if m := rackParamsToMap(nil); len(m) != 0 {
		t.Fatalf("nil params -> empty map")
	}
}

func TestResponse(t *testing.T) {
	resp := NewResponse()
	if resp.Status() != 200 {
		t.Fatalf("default status: %d", resp.Status())
	}
	resp.Write("hello ")
	resp.Write("world")
	if resp.BodyString() != "hello world" {
		t.Fatalf("body: %q", resp.BodyString())
	}
	resp.SetStatus(404)
	if resp.Status() != 404 {
		t.Fatalf("set status")
	}
}
