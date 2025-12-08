package gadm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"gadm/isdebug"
	"html/template"
	"log"
	"maps"
	"net"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"slices"
	"sync"

	"github.com/samber/lo"
	"github.com/spf13/cast"
)

func ptr[T any](t T) *T {
	return &t
}

func firstOr[T any](as []T, or ...T) T {
	if len(as) > 0 {
		return as[0]
	}
	if len(or) > 0 {
		return or[0]
	}
	var t T
	return t
}

// a != "" ? a or b
func emptyOr[T any](a, b T) T {
	if !reflect.ValueOf(a).IsZero() {
		return a
	}
	return b
}

// Ensure value avoid error/bool trouble
func must[T any](v T, lefts ...any) T {
	if len(lefts) == 0 {
		return v
	}

	// try: func() (x, error)
	if lefts[0] == nil {
		return v
	}

	if err, ok := lefts[0].(error); ok {
		panic(err)
	}

	// try: func() (x, bool)
	if err, ok := lefts[0].(bool); ok && err {
		return v
	}
	panic("type wrong")
}

// any to query in tradition url
// bool => "1", "0". behavior in python/flask
// others => string
func pyslice(as ...any) []string {
	return lo.Map(as, func(a any, _ int) string {
		if b, ok := a.(bool); ok {
			return lo.Ternary(b, "1", "0")
		}
		return cast.ToString(a)
	})
}

// Input paired args, like: a,b,c,d return "a=b&c=d"
func pairsToQuery(args ...any) url.Values {
	uv := url.Values{}
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key := cast.ToString(args[i])

			// `Add` is better than `Set`
			uv.Add(key, cast.ToString(args[i+1]))
		}
	}
	return uv
}

func queryToPairs(uv url.Values) []any {
	arr := []any{}
	for k, vs := range uv {
		for _, v := range vs {
			arr = append(arr, k, v)
		}
	}
	return arr
}

func inmap[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

// Merge b to a
func merge[K comparable, V any](a, b map[K]V) map[K]V {
	maps.Copy(a, b)
	return a
}

var (
	ContentTypeJson     = "application/json; charset=utf-8"
	ContentTypeUtf8Html = "text/html; charset=utf-8"
)

func ReplyJson(w http.ResponseWriter, status int, o any) {
	w.Header().Add("content-type", ContentTypeJson)
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(o); err != nil {
		panic(err)
	}
}

// typed nil check
func isNil(object interface{}) bool {
	if object == nil {
		return true
	}

	rv := reflect.ValueOf(object)
	return slices.Contains(
		[]reflect.Kind{reflect.Chan, reflect.Func, reflect.Interface,
			reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer},
		rv.Kind()) && rv.IsNil()
}

type cachedWriter struct {
	http.ResponseWriter
	cache      bytes.Buffer
	header     http.Header
	statusCode int
}

// Some go template actions might change headers(Cookie)
// [CacheWriter] cache body and output headers before any body
// Flush in admin.ServeHTTP
func NewCachedWriter(w http.ResponseWriter) *cachedWriter {
	return &cachedWriter{ResponseWriter: w,
		header:     http.Header{},
		statusCode: http.StatusOK,
	}
}

func (cw *cachedWriter) Header() http.Header         { return cw.header }
func (cw *cachedWriter) Write(b []byte) (int, error) { return cw.cache.Write(b) }
func (cw *cachedWriter) WriteHeader(statusCode int)  { cw.statusCode = statusCode }

// Hijack implements the http.Hijacker interface by attempting to unwrap the writer.
func (cw *cachedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	uw := cw.ResponseWriter
	// rwUnwrapper.Unwrap in NewResponseController
	if u, ok := uw.(interface{ Unwrap() http.ResponseWriter }); ok {
		uw = u.Unwrap()
	}

	if hijacker, ok := uw.(http.Hijacker); ok {
		return hijacker.Hijack()
	}

	// If the underlying writer doesn't support hijacking, use ResponseController (Go 1.20+)
	// to attempt it in a standard way, or return an error.
	rc := http.NewResponseController(cw.ResponseWriter)
	if conn, bufrw, err := rc.Hijack(); err == nil {
		return conn, bufrw, nil
	}

	// Return error if not supported
	return nil, nil, http.ErrNotSupported
}

func (cw *cachedWriter) Flush() {
	for k, vs := range cw.header {
		for _, v := range vs {
			cw.ResponseWriter.Header().Add(k, v)
		}
	}
	cw.ResponseWriter.WriteHeader(cw.statusCode)
	cw.ResponseWriter.Write(cw.cache.Bytes())
}

func withCache() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cw := NewCachedWriter(w)
			defer cw.Flush()
			next.ServeHTTP(cw, r)
		})
	}
}

// In Release, cache parsed template
// In Debug, parse every time
type groupTempl struct {
	basefn []string
	cache  sync.Map
}

func NewGroupTempl(fns ...string) *groupTempl {
	return &groupTempl{basefn: fns}
}
func (gt *groupTempl) base(funcs template.FuncMap) *template.Template {
	name := "_base"
	t, ok := gt.cache.Load(name)
	if (!ok || isdebug.On) && len(funcs) > 0 {
		t0 := must(template.New(name).
			Option("missingkey=error").
			Funcs(funcs).
			ParseFiles(gt.basefn...))
		gt.cache.Store(name, t0)
		t = t0
	}
	tpl := t.(*template.Template)
	return must(tpl.Clone())
}
func (gt *groupTempl) getOrParse(fns []string, funcs template.FuncMap) *template.Template {
	name := fns[0]
	t, ok := gt.cache.Load(name)
	if !ok || isdebug.On {
		t0 := must(gt.base(funcs).
			Option("missingkey=error").
			Funcs(funcs).
			ParseFiles(fns...))
		gt.cache.Store(name, t0)
		t = t0
	}
	return t.(*template.Template)
}

func (gt *groupTempl) Render(w http.ResponseWriter, fn string, funcs template.FuncMap, data map[string]any) error {
	tpl := gt.getOrParse([]string{fn}, funcs)
	w.Header().Add("content-type", ContentTypeUtf8Html)
	bn := path.Base(fn)
	return tpl.ExecuteTemplate(w, bn, data)
}

// call ExcuteTemplate, [name] should be valid in template set
func (gt *groupTempl) Execute(name string, data map[string]any) template.HTML {
	// assume the _base alread done
	bt := gt.base(nil)
	w := &bytes.Buffer{}
	if err := bt.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("execute %s failed: %s", name, err)
	}
	return template.HTML(w.String())
}
