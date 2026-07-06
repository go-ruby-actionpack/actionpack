// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package dispatch is the pure-Go, CGO-free analogue of ActionDispatch's HTTP
// layer: a Request and Response wrapping the reused go-ruby-rack primitives,
// augmented with routing path parameters, a merged strong-parameters view, and
// content-type format negotiation.
package dispatch

import (
	"strings"

	"github.com/go-ruby-actionpack/actionpack/parameters"
	"github.com/go-ruby-rack/rack"
)

// Request wraps a rack.Request, adding the path parameters produced by routing
// and the ActionController parameter/format accessors.
type Request struct {
	*rack.Request
	pathParams map[string]any
}

// NewRequest builds a dispatch Request over a Rack environment.
func NewRequest(env rack.Env) *Request {
	return &Request{Request: rack.NewRequest(env), pathParams: map[string]any{}}
}

// SetPathParameters stores the parameters recognized by the router (controller,
// action, and dynamic segments).
func (r *Request) SetPathParameters(p map[string]any) {
	r.pathParams = map[string]any{}
	for k, v := range p {
		r.pathParams[k] = v
	}
}

// PathParameters returns a copy of the routing path parameters.
func (r *Request) PathParameters() map[string]any {
	out := make(map[string]any, len(r.pathParams))
	for k, v := range r.pathParams {
		out[k] = v
	}
	return out
}

// ControllerName returns the controller resolved by routing, or "".
func (r *Request) ControllerName() string { return stringParam(r.pathParams, "controller") }

// ActionName returns the action resolved by routing, or "".
func (r *Request) ActionName() string { return stringParam(r.pathParams, "action") }

// QueryParameters returns the parsed query-string parameters as a plain map.
func (r *Request) QueryParameters() (map[string]any, error) {
	p, err := r.Request.GET()
	if err != nil {
		return nil, err
	}
	return rackParamsToMap(p), nil
}

// RequestParameters returns the parsed POST/body parameters as a plain map.
func (r *Request) RequestParameters() (map[string]any, error) {
	p, err := r.Request.POST()
	if err != nil {
		return nil, err
	}
	return rackParamsToMap(p), nil
}

// Parameters returns the merged strong-parameters view: query parameters,
// overlaid by request-body parameters, overlaid by routing path parameters,
// exactly matching the precedence of ActionController's params.
func (r *Request) Parameters() (*parameters.Parameters, error) {
	query, err := r.QueryParameters()
	if err != nil {
		return nil, err
	}
	body, err := r.RequestParameters()
	if err != nil {
		return nil, err
	}
	merged := map[string]any{}
	for k, v := range query {
		merged[k] = v
	}
	for k, v := range body {
		merged[k] = v
	}
	for k, v := range r.pathParams {
		merged[k] = v
	}
	return parameters.New(merged), nil
}

// Format returns the response format symbol negotiated for the request: the
// routing ":format" segment when present, otherwise the first recognised media
// type of the Accept header, defaulting to "html".
func (r *Request) Format() string {
	if f := stringParam(r.pathParams, "format"); f != "" {
		return f
	}
	return formatFromAccept(r.GetHeader("HTTP_ACCEPT"))
}

func formatFromAccept(accept string) string {
	for _, part := range strings.Split(accept, ",") {
		mt := strings.TrimSpace(part)
		if i := strings.IndexByte(mt, ';'); i >= 0 {
			mt = strings.TrimSpace(mt[:i])
		}
		switch mt {
		case "application/json", "text/json":
			return "json"
		case "application/xml", "text/xml":
			return "xml"
		case "text/html", "application/xhtml+xml":
			return "html"
		}
	}
	return "html"
}

func stringParam(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// rackParamsToMap converts a *rack.Params tree into a plain map[string]any,
// recursing through nested params and array elements.
func rackParamsToMap(p *rack.Params) map[string]any {
	out := map[string]any{}
	if p == nil {
		return out
	}
	p.Each(func(k string, v any) bool {
		out[k] = rackValueToPlain(v)
		return true
	})
	return out
}

func rackValueToPlain(v any) any {
	switch tv := v.(type) {
	case *rack.Params:
		return rackParamsToMap(tv)
	case []any:
		out := make([]any, len(tv))
		for i, el := range tv {
			out[i] = rackValueToPlain(el)
		}
		return out
	default:
		return v
	}
}
