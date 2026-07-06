// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package routing

import (
	"strings"

	"github.com/go-ruby-activesupport/activesupport/inflector"
)

// Mapper is the pure-Go analogue of ActionDispatch::Routing::Mapper: the
// receiver of the routing DSL evaluated inside RouteSet.Draw. It threads a
// lexical scope (path/module/name prefixes, constraints, defaults, and the
// enclosing resource) through nested blocks.
type Mapper struct {
	set   *RouteSet
	scope scope
	err   error
}

// scope is the accumulated lexical context of the DSL.
type scope struct {
	pathPrefix   string
	modulePrefix string // controller namespace, e.g. "admin/"
	namePrefix   string // helper-name prefix without trailing underscore
	constraints  map[string]string
	defaults     map[string]string
	resource     *resScope // enclosing resource, for member/collection/nesting
	memberWhere  string    // "member" | "collection" within Member/Collection
}

// resScope captures the current RESTful resource for member/collection routes
// and for nesting child resources.
type resScope struct {
	controller   string
	basePath     string // collection path, e.g. "/admin/posts"
	singular     string // "post"
	nameSingular string // helper base, e.g. "admin_post"
	namePlural   string // helper base, e.g. "admin_posts"
	param        string // member parameter name, e.g. "id"
	isSingular   bool   // a singular resource has no :id member segment
}

// child returns a copy of the mapper for a nested block, sharing the route set
// and error slot.
func (m *Mapper) child() *Mapper {
	c := &Mapper{set: m.set, scope: m.scope}
	c.scope.constraints = mergeMap(m.scope.constraints, nil)
	c.scope.defaults = mergeMap(m.scope.defaults, nil)
	return c
}

func mergeMap(base, extra map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// setErr records the first error only.
func (m *Mapper) setErr(err error) {
	if err != nil && m.err == nil {
		m.err = err
	}
}

// ---- functional options for plain routes -------------------------------

type routeOpts struct {
	to          string
	as          string
	controller  string
	action      string
	via         []string
	constraints map[string]string
	defaults    map[string]string
	on          string // "member" | "collection" | ""
}

// Option customises a plain route (get/post/match).
type Option func(*routeOpts)

// To sets the "controller#action" dispatch target.
func To(target string) Option { return func(o *routeOpts) { o.to = target } }

// As sets the path-helper name.
func As(name string) Option { return func(o *routeOpts) { o.as = name } }

// Controller sets the controller explicitly (used with Action).
func Controller(name string) Option { return func(o *routeOpts) { o.controller = name } }

// Action sets the action explicitly (used with Controller).
func Action(name string) Option { return func(o *routeOpts) { o.action = name } }

// Via sets the HTTP methods a match route responds to.
func Via(methods ...string) Option { return func(o *routeOpts) { o.via = methods } }

// Constraints sets per-segment regexp requirements (raw, unanchored sources).
func Constraints(reqs map[string]string) Option {
	return func(o *routeOpts) { o.constraints = reqs }
}

// Defaults sets default parameters merged into recognitions of the route.
func Defaults(defs map[string]string) Option {
	return func(o *routeOpts) { o.defaults = defs }
}

// On places a resource route at the :member or :collection level.
func On(where string) Option { return func(o *routeOpts) { o.on = where } }

// ---- plain route methods ----------------------------------------------

// Get adds a GET route. See Match for option handling.
func (m *Mapper) Get(path string, opts ...Option) { m.matchVerb("GET", path, opts) }

// Post adds a POST route.
func (m *Mapper) Post(path string, opts ...Option) { m.matchVerb("POST", path, opts) }

// Put adds a PUT route.
func (m *Mapper) Put(path string, opts ...Option) { m.matchVerb("PUT", path, opts) }

// Patch adds a PATCH route.
func (m *Mapper) Patch(path string, opts ...Option) { m.matchVerb("PATCH", path, opts) }

// Delete adds a DELETE route.
func (m *Mapper) Delete(path string, opts ...Option) { m.matchVerb("DELETE", path, opts) }

// Match adds a route responding to the methods given by Via (or any method when
// none are supplied).
func (m *Mapper) Match(path string, opts ...Option) { m.matchVerb("", path, opts) }

func (m *Mapper) matchVerb(verb, path string, opts []Option) {
	o := &routeOpts{}
	for _, fn := range opts {
		fn(o)
	}
	if m.scope.resource != nil {
		where := o.on
		if where == "" {
			where = m.scope.memberWhere
		}
		if where == "" {
			where = "member" // a bare route inside a resource is a member route
		}
		m.addResourceMember(where, verb, path, o)
		return
	}
	verbs := []string{verb}
	if verb == "" {
		verbs = o.via
		if len(verbs) == 0 {
			verbs = []string{""}
		}
	}
	controller, action := m.resolveTarget(o, defaultActionFromPath(path))
	name := o.as
	spec := m.scope.pathPrefix + normalizeLeading(path)
	reqs := mergeMap(m.scope.constraints, o.constraints)
	defs := mergeMap(m.scope.defaults, o.defaults)
	for _, v := range verbs {
		nm := name
		if v != verbs[0] {
			nm = "" // only the first verb owns the helper name
		}
		m.setErr(m.set.addRoute(nm, v, withFormat(spec), controller, action, defs, reqs))
	}
}

// addResourceMember handles get "x", on: :member/:collection inside a resource.
func (m *Mapper) addResourceMember(where, verb, path string, o *routeOpts) {
	rs := m.scope.resource
	action := lastSegment(path)
	if o.to != "" {
		_, action = splitTarget(o.to)
	}
	var spec, name string
	if where == "collection" || rs.isSingular {
		// A singular resource has no :id, so its member routes live at the
		// collection level.
		spec = rs.basePath + "/" + path
		name = joinName(action, rs.namePlural)
		if o.as != "" {
			name = joinName(o.as, rs.namePlural)
		}
	} else {
		spec = rs.basePath + "/:" + rs.param + "/" + path
		name = joinName(action, rs.nameSingular)
		if o.as != "" {
			name = joinName(o.as, rs.nameSingular)
		}
	}
	reqs := mergeMap(m.scope.constraints, o.constraints)
	defs := mergeMap(m.scope.defaults, o.defaults)
	m.setErr(m.set.addRoute(name, verb, withFormat(spec), rs.controller, action, defs, reqs))
}

// resolveTarget derives controller and action for a plain (non-resource) route
// from its options: an explicit "controller#action" target, or a Controller
// option combined with an Action option (defaulting the action to the last path
// segment).
func (m *Mapper) resolveTarget(o *routeOpts, defaultAction string) (string, string) {
	if o.to != "" {
		c, a := splitTarget(o.to)
		return m.scope.modulePrefix + c, a
	}
	action := o.action
	if action == "" {
		action = defaultAction
	}
	return m.scope.modulePrefix + o.controller, action
}

// Root adds the root route (GET "/") named "root".
func (m *Mapper) Root(target string, opts ...Option) {
	o := &routeOpts{}
	for _, fn := range opts {
		fn(o)
	}
	c, a := splitTarget(target)
	name := joinName("root", m.scope.namePrefix)
	spec := m.scope.pathPrefix
	if spec == "" {
		spec = "/"
	}
	reqs := mergeMap(m.scope.constraints, o.constraints)
	defs := mergeMap(m.scope.defaults, o.defaults)
	m.setErr(m.set.addRoute(name, "GET", spec, m.scope.modulePrefix+c, a, defs, reqs))
}

// ---- scope, namespace, constraints ------------------------------------

// ScopeOpts are the prefixes and options a Scope block applies.
type ScopeOpts struct {
	Path        string
	Module      string
	As          string
	Constraints map[string]string
	Defaults    map[string]string
}

// Scope evaluates block within an added path/module/name scope.
func (m *Mapper) Scope(opts ScopeOpts, block func(*Mapper)) {
	c := m.child()
	if opts.Path != "" {
		c.scope.pathPrefix += normalizeLeading(opts.Path)
	}
	if opts.Module != "" {
		c.scope.modulePrefix += opts.Module + "/"
	}
	if opts.As != "" {
		c.scope.namePrefix = joinName(c.scope.namePrefix, opts.As)
	}
	c.scope.constraints = mergeMap(c.scope.constraints, opts.Constraints)
	c.scope.defaults = mergeMap(c.scope.defaults, opts.Defaults)
	block(c)
	m.setErr(c.err)
}

// Namespace evaluates block under a path, module, and name prefix all equal to
// name (namespace :admin).
func (m *Mapper) Namespace(name string, block func(*Mapper)) {
	m.Scope(ScopeOpts{Path: name, Module: name, As: name}, block)
}

// ConstraintsBlock evaluates block with added segment constraints.
func (m *Mapper) ConstraintsBlock(reqs map[string]string, block func(*Mapper)) {
	m.Scope(ScopeOpts{Constraints: reqs}, block)
}

// Member evaluates block adding routes at the :member level of the enclosing
// resource (/resource/:id/action).
func (m *Mapper) Member(block func(*Mapper)) { m.memberScope("member", block) }

// Collection evaluates block adding routes at the :collection level of the
// enclosing resource (/resource/action).
func (m *Mapper) Collection(block func(*Mapper)) { m.memberScope("collection", block) }

func (m *Mapper) memberScope(where string, block func(*Mapper)) {
	if m.scope.resource == nil {
		return
	}
	c := m.child()
	c.scope.memberWhere = where
	block(c)
	m.setErr(c.err)
}

// ---- resources / resource ---------------------------------------------

type resConfig struct {
	only       []string
	except     []string
	pathName   string
	controller string
	param      string
}

// ResOption customises a resources/resource declaration.
type ResOption func(*resConfig)

// Only restricts the generated RESTful actions to those listed.
func Only(actions ...string) ResOption { return func(c *resConfig) { c.only = actions } }

// Except omits the listed RESTful actions.
func Except(actions ...string) ResOption { return func(c *resConfig) { c.except = actions } }

// PathName overrides the URL path segment (resources :posts, path: "articles").
func PathName(name string) ResOption { return func(c *resConfig) { c.pathName = name } }

// ResController overrides the controller for the resource.
func ResController(name string) ResOption { return func(c *resConfig) { c.controller = name } }

// ResParam overrides the member parameter name (default "id").
func ResParam(name string) ResOption { return func(c *resConfig) { c.param = name } }

// resRoute is a template for one generated RESTful route.
type resRoute struct {
	method string
	suffix string // path after the base, e.g. "/new" or "/:id"
	action string
	name   string // full helper name, or "" for none
}

// Resources declares a plural RESTful resource (7 routes). block, when non-nil,
// receives a mapper scoped to the resource for member/collection/nested routes.
func (m *Mapper) Resources(name string, block func(*Mapper), opts ...ResOption) {
	cfg := resConfig{param: "id"}
	for _, fn := range opts {
		fn(&cfg)
	}
	rs, pathPrefix, namePrefix := m.buildResScope(name, inflector.Singularize(name), name, &cfg, false)
	routes := []resRoute{
		{"GET", "", "index", rs.namePlural},
		{"POST", "", "create", ""},
		{"GET", "/new", "new", joinName("new", rs.nameSingular)},
		{"GET", "/:" + cfg.param + "/edit", "edit", joinName("edit", rs.nameSingular)},
		{"GET", "/:" + cfg.param, "show", rs.nameSingular},
		{"PATCH", "/:" + cfg.param, "update", ""},
		{"PUT", "/:" + cfg.param, "update", ""},
		{"DELETE", "/:" + cfg.param, "destroy", ""},
	}
	// Rails evaluates the resource block before adding the default RESTful
	// routes, so custom collection/member routes precede the dynamic :id route.
	m.runResourceBlock(block, rs, pathPrefix, namePrefix)
	m.emitResourceRoutes(rs, routes, &cfg)
}

// Resource declares a singular RESTful resource (6 routes, no index, no :id).
func (m *Mapper) Resource(name string, block func(*Mapper), opts ...ResOption) {
	cfg := resConfig{param: "id"}
	for _, fn := range opts {
		fn(&cfg)
	}
	controllerName := inflector.Pluralize(name)
	rs, pathPrefix, namePrefix := m.buildResScope(name, name, controllerName, &cfg, true)
	routes := []resRoute{
		{"GET", "/new", "new", joinName("new", rs.nameSingular)},
		{"POST", "", "create", ""},
		{"GET", "", "show", rs.nameSingular},
		{"GET", "/edit", "edit", joinName("edit", rs.nameSingular)},
		{"PATCH", "", "update", ""},
		{"PUT", "", "update", ""},
		{"DELETE", "", "destroy", ""},
	}
	m.runResourceBlock(block, rs, pathPrefix, namePrefix)
	m.emitResourceRoutes(rs, routes, &cfg)
}

// buildResScope computes the resScope for a resource, honouring nesting under an
// enclosing resource. It returns the scope plus the path/name prefixes for the
// resource's block.
func (m *Mapper) buildResScope(name, singular, defaultController string, cfg *resConfig, singularRes bool) (*resScope, string, string) {
	pathPrefix := m.scope.pathPrefix
	namePrefix := m.scope.namePrefix
	if p := m.scope.resource; p != nil {
		pathPrefix = p.basePath + "/:" + p.singular + "_id"
		namePrefix = p.nameSingular
	}
	pathSeg := name
	if cfg.pathName != "" {
		pathSeg = cfg.pathName
	}
	controllerBase := defaultController
	if cfg.controller != "" {
		controllerBase = cfg.controller
	}
	rs := &resScope{
		controller:   m.scope.modulePrefix + controllerBase,
		basePath:     pathPrefix + "/" + pathSeg,
		singular:     singular,
		nameSingular: joinName(namePrefix, singular),
		namePlural:   joinName(namePrefix, name),
		param:        cfg.param,
	}
	if singularRes {
		// A singular resource shares its name for both singular and plural
		// helper positions (there is no index).
		rs.namePlural = rs.nameSingular
		rs.isSingular = true
	}
	return rs, pathPrefix, namePrefix
}

// emitResourceRoutes filters the templates by only/except and registers them.
func (m *Mapper) emitResourceRoutes(rs *resScope, routes []resRoute, cfg *resConfig) {
	reqs := mergeMap(m.scope.constraints, nil)
	defs := mergeMap(m.scope.defaults, nil)
	for _, r := range routes {
		if !actionAllowed(r.action, cfg) {
			continue
		}
		spec := withFormat(rs.basePath + r.suffix)
		m.setErr(m.set.addRoute(r.name, r.method, spec, rs.controller, r.action, defs, reqs))
	}
}

// runResourceBlock evaluates a resource's block with the resource in scope.
func (m *Mapper) runResourceBlock(block func(*Mapper), rs *resScope, pathPrefix, namePrefix string) {
	if block == nil {
		return
	}
	c := m.child()
	c.scope.resource = rs
	c.scope.pathPrefix = pathPrefix
	c.scope.namePrefix = namePrefix
	block(c)
	m.setErr(c.err)
}

// actionAllowed applies the only/except filters to a RESTful action.
func actionAllowed(action string, cfg *resConfig) bool {
	if len(cfg.only) > 0 {
		return contains(cfg.only, action)
	}
	if len(cfg.except) > 0 {
		return !contains(cfg.except, action)
	}
	return true
}

// ---- helpers ----------------------------------------------------------

func joinName(prefix, base string) string {
	switch {
	case prefix == "":
		return base
	case base == "":
		return prefix
	default:
		return prefix + "_" + base
	}
}

func splitTarget(target string) (controller, action string) {
	if i := strings.LastIndex(target, "#"); i >= 0 {
		return target[:i], target[i+1:]
	}
	return target, ""
}

func defaultActionFromPath(path string) string {
	return lastSegment(path)
}

func lastSegment(path string) string {
	p := strings.Trim(path, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	if j := strings.IndexByte(p, '('); j >= 0 {
		p = p[:j]
	}
	return p
}

func normalizeLeading(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

// withFormat appends the optional "(.:format)" suffix used by Rails on all
// resourceful and matched routes.
func withFormat(spec string) string {
	return spec + "(.:format)"
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
