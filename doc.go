// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package actionpack is the root of go-ruby-actionpack, a pure-Go, CGO-free
// port of Rails' ActionPack faithful to MRI/Rails 4.0.5-era semantics, for use
// on its own and as the substrate of a future rbgo binding.
//
// The library is organised into subpackages mirroring the ActionPack
// namespaces:
//
//   - routing: ActionDispatch::Routing — the routing DSL, the compiled route
//     set, request recognition, and reverse-routing path helpers.
//   - controller: AbstractController::Base / ActionController::Metal — the
//     dispatch core with filters, rescue_from, and render/redirect/head seams.
//   - parameters: ActionController::Parameters — strong parameters.
//   - dispatch: ActionDispatch's Request/Response over the reused go-ruby-rack
//     primitives, with path parameters, merged params, and format negotiation.
//
// See the package documentation of each subpackage and the repository README
// for the v0.1 scope and the roadmap of deferred functionality (view
// rendering, sessions/cookies/flash, CSRF, caching, and the full Journey
// constraint engine).
package actionpack
