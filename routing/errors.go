// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package routing

// RouteNotFound is returned by Path/PathArgs when no route carries the
// requested helper name.
type RouteNotFound struct{ Name string }

func (e *RouteNotFound) Error() string {
	return "routing: no named route " + e.Name
}

// MissingRouteKeys is returned by generation when required dynamic segments
// have no value.
type MissingRouteKeys struct {
	Name string
	Spec string
}

func (e *MissingRouteKeys) Error() string {
	return "routing: missing required keys to generate " + e.Spec
}

// UnroutableParameters is returned by UrlFor when no route matches the given
// controller/action.
type UnroutableParameters struct {
	Controller string
	Action     string
}

func (e *UnroutableParameters) Error() string {
	return "routing: no route matches controller=" + e.Controller +
		" action=" + e.Action
}
