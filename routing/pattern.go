// Copyright (c) the go-ruby-actionpack/actionpack authors
//
// SPDX-License-Identifier: BSD-3-Clause

package routing

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// A pattern is a compiled path specification, the pure-Go analogue of the
// Journey pattern behind an ActionDispatch route. It supports:
//
//   - static segments ("/posts");
//   - dynamic segments (":id"), matching one path component by default
//     ([^/.?]+) or a caller-supplied requirement;
//   - globbing segments ("*path"), matching the remainder including slashes;
//   - optional groups ("(...)"), typically the "(.:format)" suffix.
//
// A pattern both recognises a path (match) and reverses back to one (generate).
type pattern struct {
	spec     string
	nodes    []node
	re       *regexp.Regexp
	required []string // dynamic names outside any optional group, in order
}

// node is one element of a parsed path specification.
type node interface{ isNode() }

type litNode struct{ text string }
type symNode struct{ name string }
type starNode struct{ name string }
type groupNode struct{ children []node }

func (litNode) isNode()   {}
func (symNode) isNode()   {}
func (starNode) isNode()  {}
func (groupNode) isNode() {}

// specParser is a small recursive-descent parser over the path specification.
type specParser struct {
	r   []rune
	pos int
}

// compilePattern parses spec and compiles its matching regexp. requirements
// maps a dynamic segment name to a raw (unanchored) regexp source overriding
// the default component matcher.
func compilePattern(spec string, requirements map[string]string) (*pattern, error) {
	p := &specParser{r: []rune(spec)}
	nodes, err := p.parseSeq(false)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(`\A`)
	writeRegexp(&b, nodes, requirements)
	b.WriteString(`\z`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, err
	}
	pat := &pattern{spec: spec, nodes: nodes, re: re}
	pat.required = requiredNames(nodes)
	return pat, nil
}

func (p *specParser) peek() (rune, bool) {
	if p.pos >= len(p.r) {
		return 0, false
	}
	return p.r[p.pos], true
}

// parseSeq parses a sequence of nodes. When inGroup is true it stops at the
// closing ')', which the caller consumes.
func (p *specParser) parseSeq(inGroup bool) ([]node, error) {
	var nodes []node
	for {
		c, ok := p.peek()
		if !ok {
			if inGroup {
				return nil, fmt.Errorf("routing: unbalanced '(' in %q", string(p.r))
			}
			return nodes, nil
		}
		switch c {
		case ')':
			if !inGroup {
				return nil, fmt.Errorf("routing: unexpected ')' in %q", string(p.r))
			}
			return nodes, nil
		case '(':
			p.pos++
			children, err := p.parseSeq(true)
			if err != nil {
				return nil, err
			}
			// parseSeq(true) only returns without error when positioned at the
			// closing ')', which we consume here.
			p.pos++ // consume ')'
			nodes = append(nodes, groupNode{children: children})
		case ':':
			p.pos++
			name, err := p.readName()
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, symNode{name: name})
		case '*':
			p.pos++
			name, err := p.readName()
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, starNode{name: name})
		default:
			nodes = append(nodes, litNode{text: p.readLiteral()})
		}
	}
}

// readName reads a segment name [A-Za-z0-9_]+.
func (p *specParser) readName() (string, error) {
	start := p.pos
	for p.pos < len(p.r) && isNameRune(p.r[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return "", fmt.Errorf("routing: empty segment name in %q", string(p.r))
	}
	return string(p.r[start:p.pos]), nil
}

// readLiteral reads a run of literal characters up to the next special rune.
func (p *specParser) readLiteral() string {
	start := p.pos
	for p.pos < len(p.r) {
		c := p.r[p.pos]
		if c == '(' || c == ')' || c == ':' || c == '*' {
			break
		}
		p.pos++
	}
	return string(p.r[start:p.pos])
}

func isNameRune(c rune) bool {
	return c == '_' ||
		(c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9')
}

// writeRegexp emits the matching regexp for nodes into b. Any per-segment
// requirement is embedded raw; an invalid one surfaces later as a
// regexp.Compile error in compilePattern.
func writeRegexp(b *strings.Builder, nodes []node, reqs map[string]string) {
	for _, n := range nodes {
		switch v := n.(type) {
		case litNode:
			b.WriteString(regexp.QuoteMeta(v.text))
		case symNode:
			sub := defaultSegment
			if r, ok := reqs[v.name]; ok {
				sub = "(?:" + r + ")"
			}
			b.WriteString("(?P<" + v.name + ">" + sub + ")")
		case starNode:
			sub := ".+"
			if r, ok := reqs[v.name]; ok {
				sub = "(?:" + r + ")"
			}
			b.WriteString("(?P<" + v.name + ">" + sub + ")")
		case groupNode:
			b.WriteString("(?:")
			writeRegexp(b, v.children, reqs)
			b.WriteString(")?")
		}
	}
}

// defaultSegment is the component matcher for a dynamic segment: one or more
// characters that are not a slash, dot, or question mark.
const defaultSegment = `[^/.?]+`

// requiredNames returns dynamic segment names that live outside any optional
// group, in order; these are the positional arguments of a path helper.
func requiredNames(nodes []node) []string {
	var out []string
	for _, n := range nodes {
		switch v := n.(type) {
		case symNode:
			out = append(out, v.name)
		case starNode:
			out = append(out, v.name)
		}
	}
	return out
}

// match tests path against the pattern, returning the captured dynamic segments
// (only those that participated) and whether it matched.
func (pat *pattern) match(path string) (map[string]string, bool) {
	idx := pat.re.FindStringSubmatchIndex(path)
	if idx == nil {
		return nil, false
	}
	names := pat.re.SubexpNames()
	params := map[string]string{}
	for i, name := range names {
		if name == "" {
			continue
		}
		start := idx[2*i]
		if start < 0 {
			continue // optional group that did not participate
		}
		params[name] = path[start:idx[2*i+1]]
	}
	return params, true
}

// generate reverses the pattern into a path using params, marking every
// consumed name in consumed. It returns ok=false when a required (non-optional)
// segment has no value.
func (pat *pattern) generate(params map[string]any, consumed map[string]bool) (string, bool) {
	return genNodes(pat.nodes, params, consumed)
}

func genNodes(nodes []node, params map[string]any, consumed map[string]bool) (string, bool) {
	var b strings.Builder
	for _, n := range nodes {
		switch v := n.(type) {
		case litNode:
			b.WriteString(v.text)
		case symNode:
			s, ok := toParam(params[v.name])
			if _, present := params[v.name]; !present || !ok {
				return "", false
			}
			b.WriteString(s)
			consumed[v.name] = true
		case starNode:
			s, ok := toParam(params[v.name])
			if _, present := params[v.name]; !present || !ok {
				return "", false
			}
			b.WriteString(s)
			consumed[v.name] = true
		case groupNode:
			local := map[string]bool{}
			sub, ok := genNodes(v.children, params, local)
			if ok {
				b.WriteString(sub)
				for k := range local {
					consumed[k] = true
				}
			}
		}
	}
	return b.String(), true
}

// toParam converts a segment value to its string form (Ruby to_param), ok=false
// for an empty/blank value that cannot fill a required segment.
func toParam(v any) (string, bool) {
	switch tv := v.(type) {
	case nil:
		return "", false
	case string:
		return tv, tv != ""
	case bool:
		return strconv.FormatBool(tv), true
	case int:
		return strconv.Itoa(tv), true
	case int64:
		return strconv.FormatInt(tv, 10), true
	case fmt.Stringer:
		s := tv.String()
		return s, s != ""
	default:
		s := fmt.Sprint(tv)
		return s, s != ""
	}
}
