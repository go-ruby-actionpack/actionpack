// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package routing

import (
	"fmt"
	"net/url"
	"sort"
)

// RouteSet is the pure-Go analogue of ActionDispatch::Routing::RouteSet: an
// ordered collection of routes with reverse-routing helpers. Recognition scans
// routes in definition order and returns the first match, exactly like Rails.
type RouteSet struct {
	routes []*Route
	named  map[string]*Route
}

// NewRouteSet returns an empty route set.
func NewRouteSet() *RouteSet {
	return &RouteSet{named: map[string]*Route{}}
}

// Draw evaluates the routing DSL in fn against a fresh Mapper and returns the
// first error encountered while compiling any path pattern.
func (rs *RouteSet) Draw(fn func(*Mapper)) error {
	m := &Mapper{set: rs}
	fn(m)
	return m.err
}

// Routes returns the routes in definition order. The slice is a copy.
func (rs *RouteSet) Routes() []*Route {
	out := make([]*Route, len(rs.routes))
	copy(out, rs.routes)
	return out
}

// Recognition is the outcome of recognizing a request: the matched route and
// the resolved controller, action, and path parameters.
type Recognition struct {
	Route      *Route
	Controller string
	Action     string
	// Params holds the merged defaults and captured path segments (including
	// "controller", "action", and, when present, "format").
	Params map[string]any
}

// addRoute compiles spec and appends the resulting route. The first compile
// failure is remembered on the mapper via the returned error.
func (rs *RouteSet) addRoute(name, verb, spec, controller, action string, defaults, requirements map[string]string) error {
	pat, err := compilePattern(spec, requirements)
	if err != nil {
		return err
	}
	d := map[string]string{"controller": controller, "action": action}
	for k, v := range defaults {
		d[k] = v
	}
	r := &Route{
		Name:       name,
		Verb:       verb,
		Spec:       spec,
		Controller: controller,
		Action:     action,
		Defaults:   d,
		pat:        pat,
	}
	rs.routes = append(rs.routes, r)
	if name != "" {
		rs.named[name] = r
	}
	return nil
}

// Recognize maps an HTTP method and path to a controller, action, and
// parameters, returning ok=false when nothing matches.
func (rs *RouteSet) Recognize(method, path string) (*Recognition, bool) {
	for _, r := range rs.routes {
		if !r.verbMatches(method) {
			continue
		}
		caps, ok := r.pat.match(path)
		if !ok {
			continue
		}
		params := map[string]any{}
		for k, v := range r.Defaults {
			params[k] = v
		}
		for k, v := range caps {
			params[k] = v
		}
		return &Recognition{
			Route:      r,
			Controller: asString(params["controller"]),
			Action:     asString(params["action"]),
			Params:     params,
		}, true
	}
	return nil, false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Path generates the path for the named route with the given parameters. Extra
// parameters that are not path segments become a sorted query string.
func (rs *RouteSet) Path(name string, params map[string]any) (string, error) {
	r, ok := rs.named[name]
	if !ok {
		return "", &RouteNotFound{Name: name}
	}
	return rs.generate(r, params)
}

// PathArgs generates the path for the named route from positional arguments,
// mapped to the route's required dynamic segments in order (post_path(1)). A
// trailing map[string]any argument supplies extra options (e.g. format or
// query parameters).
func (rs *RouteSet) PathArgs(name string, args ...any) (string, error) {
	r, ok := rs.named[name]
	if !ok {
		return "", &RouteNotFound{Name: name}
	}
	params := map[string]any{}
	if n := len(args); n > 0 {
		if extra, isMap := args[n-1].(map[string]any); isMap {
			for k, v := range extra {
				params[k] = v
			}
			args = args[:n-1]
		}
	}
	req := r.pat.required
	if len(args) != len(req) {
		return "", fmt.Errorf("routing: %s expects %d positional argument(s), got %d",
			name, len(req), len(args))
	}
	for i, key := range req {
		params[key] = args[i]
	}
	return rs.generate(r, params)
}

// UrlFor performs reverse routing from an options map containing "controller"
// and "action" (plus any dynamic segment values), returning the path of the
// first route whose target and required segments match.
func (rs *RouteSet) UrlFor(opts map[string]any) (string, error) {
	controller := asString(opts["controller"])
	action := asString(opts["action"])
	for _, r := range rs.routes {
		if r.Controller != controller || r.Action != action {
			continue
		}
		consumed := map[string]bool{}
		path, ok := r.pat.generate(opts, consumed)
		if !ok {
			continue
		}
		return appendQuery(path, opts, consumed), nil
	}
	return "", &UnroutableParameters{Controller: controller, Action: action}
}

// generate reverses a route into a path, appending leftover params as a query.
func (rs *RouteSet) generate(r *Route, params map[string]any) (string, error) {
	consumed := map[string]bool{}
	path, ok := r.pat.generate(params, consumed)
	if !ok {
		return "", &MissingRouteKeys{Name: r.Name, Spec: r.Spec}
	}
	return appendQuery(path, params, consumed), nil
}

// appendQuery appends the parameters not consumed by the path (and not the
// controller/action routing keys) as a sorted URL query string.
func appendQuery(path string, params map[string]any, consumed map[string]bool) string {
	var keys []string
	for k := range params {
		if consumed[k] || k == "controller" || k == "action" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return path
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		s, ok := toParam(params[k])
		if !ok {
			continue
		}
		pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(s))
	}
	if len(pairs) == 0 {
		return path
	}
	q := pairs[0]
	for _, p := range pairs[1:] {
		q += "&" + p
	}
	return path + "?" + q
}
