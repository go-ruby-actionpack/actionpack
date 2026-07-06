// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package routing

import "strings"

// Route is one compiled routing rule: an HTTP verb, a path pattern, and the
// controller/action it dispatches to, plus any default parameters and a
// reverse-routing name (the base of its path helper, e.g. "post" for
// post_path).
type Route struct {
	// Name is the path-helper base, or "" when the route adds no new helper.
	Name string
	// Verb is the upper-case HTTP method, or "" to match any method.
	Verb string
	// Spec is the raw path specification (e.g. "/posts/:id(.:format)").
	Spec string
	// Controller and Action are the dispatch target.
	Controller string
	Action     string
	// Defaults are parameters merged into every recognition of this route
	// (they include "controller" and "action").
	Defaults map[string]string

	pat *pattern
}

// verbMatches reports whether method should be routed by this route. HEAD is
// treated as GET, matching Rack/Rails.
func (r *Route) verbMatches(method string) bool {
	if r.Verb == "" {
		return true
	}
	m := strings.ToUpper(method)
	if m == "HEAD" {
		m = "GET"
	}
	return r.Verb == m
}
