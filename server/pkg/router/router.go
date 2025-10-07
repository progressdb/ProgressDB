package router

import (
	"strings"

	"github.com/valyala/fasthttp"
)

// Router is a minimal HTTP router compatible with a subset of
// progressdb/pkg/router. It supports parameterised paths using {name} and
// dispatches handlers by HTTP method.
type Router struct {
	routes   map[string][]route
	notFound fasthttp.RequestHandler
}

type route struct {
	segments []segment
	handler  fasthttp.RequestHandler
}

type segment struct {
	name    string
	isParam bool
}

// New constructs a new Router.
func New() *Router {
	return &Router{routes: make(map[string][]route)}
}

// Handler satisfies the fasthttp.Server handler interface.
func (r *Router) Handler(ctx *fasthttp.RequestCtx) {
	method := string(ctx.Method())
	path := string(ctx.Path())
	if list, ok := r.routes[method]; ok {
		for _, rt := range list {
			if values, ok := match(path, rt.segments); ok {
				for k, v := range values {
					ctx.SetUserValue(k, v)
				}
				rt.handler(ctx)
				return
			}
		}
	}
	if r.notFound != nil {
		r.notFound(ctx)
		return
	}
	ctx.SetStatusCode(fasthttp.StatusNotFound)
}

// GET registers a GET handler.
func (r *Router) GET(path string, h fasthttp.RequestHandler) {
	r.add("GET", path, h)
}

// POST registers a POST handler.
func (r *Router) POST(path string, h fasthttp.RequestHandler) {
	r.add("POST", path, h)
}

// PUT registers a PUT handler.
func (r *Router) PUT(path string, h fasthttp.RequestHandler) {
	r.add("PUT", path, h)
}

// DELETE registers a DELETE handler.
func (r *Router) DELETE(path string, h fasthttp.RequestHandler) {
	r.add("DELETE", path, h)
}

// NotFound registers a handler for unmatched routes.
func (r *Router) NotFound(h fasthttp.RequestHandler) {
	r.notFound = h
}

func (r *Router) add(method, path string, h fasthttp.RequestHandler) {
	segments := parse(path)
	r.routes[method] = append(r.routes[method], route{segments: segments, handler: h})
}

func parse(path string) []segment {
	if path == "" {
		return nil
	}
	if path[0] == '/' {
		path = path[1:]
	}
	if path == "" {
		return []segment{{name: "", isParam: false}}
	}
	parts := strings.Split(path, "/")
	segs := make([]segment, len(parts))
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") && len(part) > 2 {
			segs[i] = segment{name: part[1 : len(part)-1], isParam: true}
		} else {
			segs[i] = segment{name: part, isParam: false}
		}
	}
	return segs
}

func match(path string, segs []segment) (map[string]string, bool) {
	if len(segs) == 1 && !segs[0].isParam && segs[0].name == "" {
		if path == "/" || path == "" {
			return map[string]string{}, true
		}
		return nil, false
	}
	if path == "" {
		path = "/"
	}
	if path[0] == '/' {
		path = path[1:]
	}
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	if len(parts) != len(segs) {
		return nil, false
	}
	values := make(map[string]string)
	for i, seg := range segs {
		if seg.isParam {
			values[seg.name] = parts[i]
			continue
		}
		if seg.name != parts[i] {
			return nil, false
		}
	}
	return values, true
}
