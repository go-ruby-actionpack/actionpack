// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package controller

// RenderOptions describes a render call. The actual template/view rendering is
// deferred to a later ActionView port; without a Renderer seam only the Plain
// body is written. Options mirror the common ActionController render keys.
type RenderOptions struct {
	// Template names a view template ("posts/show").
	Template string
	// Action names an action whose template to render (defaults to the
	// current action when a Renderer resolves templates).
	Action string
	// Plain is an inline text/plain body.
	Plain string
	// JSON, when non-nil, is a value the Renderer serialises as JSON.
	JSON any
	// Status is the HTTP status (defaults to 200).
	Status int
	// ContentType overrides the response content type.
	ContentType string
	// Layout names the layout template, for the view seam.
	Layout string
}

// Render writes a response body per opts, marking the controller performed. A
// Renderer seam, if installed, produces the body; otherwise the Plain option is
// used. Rendering twice returns *DoubleRenderError.
func (b *Base) Render(opts RenderOptions) error {
	if err := b.ensurePerformable(); err != nil {
		return err
	}
	body := opts.Plain
	if b.class.renderer != nil {
		out, err := b.class.renderer(b, opts)
		if err != nil {
			return err
		}
		body = out
	}
	status := opts.Status
	if status == 0 {
		status = 200
	}
	b.response.SetStatus(status)
	if opts.ContentType != "" {
		b.response.SetContentType(opts.ContentType)
	}
	b.response.Write(body)
	o := opts
	b.Rendered = &o
	b.performed = true
	return nil
}

// RenderPlain is the shorthand for Render(RenderOptions{Plain: text}).
func (b *Base) RenderPlain(text string) error {
	return b.Render(RenderOptions{Plain: text, ContentType: "text/plain"})
}

// RedirectTo sets a Location header and a redirect status (302 by default),
// marking the controller performed. Redirecting after performing returns
// *DoubleRenderError.
func (b *Base) RedirectTo(url string, status ...int) error {
	if err := b.ensurePerformable(); err != nil {
		return err
	}
	st := 302
	if len(status) > 0 {
		st = status[0]
	}
	b.response.Redirect(url, st)
	b.performed = true
	b.RedirectedTo = url
	return nil
}

// Head sets an empty response with the given status, marking the controller
// performed. Performing twice returns *DoubleRenderError.
func (b *Base) Head(status int) error {
	if err := b.ensurePerformable(); err != nil {
		return err
	}
	b.response.SetStatus(status)
	b.performed = true
	return nil
}

// ensurePerformable guards against a double render/redirect.
func (b *Base) ensurePerformable() error {
	if b.performed {
		return &DoubleRenderError{}
	}
	return nil
}
