// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package dispatch

import "github.com/go-ruby-rack/rack"

// Response wraps a rack.Response with ActionController conveniences. It starts
// as an empty 200 response; the embedded *rack.Response promotes Write, Status,
// SetStatus, Headers, SetContentType, SetLocation, and the rest of the Rack
// response API.
type Response struct {
	*rack.Response
}

// NewResponse returns an empty 200 response.
func NewResponse() *Response {
	return &Response{Response: rack.NewResponse([]string{}, 200, rack.NewHeaders())}
}

// BodyString returns the response body joined into a single string.
func (r *Response) BodyString() string {
	var b string
	for _, chunk := range r.Response.Body() {
		b += chunk
	}
	return b
}
