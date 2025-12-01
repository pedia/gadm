package gadm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gadm/isdebug"
	"html/template"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/Masterminds/sprig/v3"
	"github.com/gorilla/csrf"
	"github.com/gorilla/handlers"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"github.com/samber/lo"
	"gopkg.in/leonelquinteros/gotext.v1"

	"gorm.io/gorm"
)

func key(secret string) []byte {
	h := sha256.New()
	h.Write([]byte(secret))
	return h.Sum(nil)
}

func NewAdmin(name string) *Admin {
	key := key("hello") // TODO: read from config
	A := &Admin{
		BaseView: NewView(Menu{
			Path: "/admin/",
			Name: gettext("Home"),
		}),
		views:       []View{},
		dbs:         map[string]*gorm.DB{},
		debug:       isdebug.On,
		autoMigrate: true,
		trace:       true,
		tracer:      NewTrace(),
		key:         key,
		sessionKey:  "sess",
		store:       sessions.NewCookieStore(key),
		csrf: csrf.Protect(key,
			csrf.CookieName("csrf"), csrf.FieldName("csrf_token")),
		mux: http.NewServeMux(),

		indexTemplateFile: "templates/index.tmpl",
		theme:             "default", // "cyborg",
	}
	A.BaseView.admin = A

	A.Blueprint = &Blueprint{
		Name:     name,
		Endpoint: "admin",
		Path:     "/admin",
		Handler:  A.indexHandler,
		Children: map[string]*Blueprint{
			"index":      {Endpoint: "index", Path: "/", Handler: A.indexHandler},
			"debug":      {Endpoint: "debug", Path: "/debug.json", Handler: A.debugHandler},
			"debug.html": {Endpoint: "debug.html", Path: "/debug.html", Handler: A.debugHtmlHandler},
			"generate":   {Endpoint: "generate", Path: "/generate", Handler: A.generateHandler},
			"console":    {Endpoint: "console", Path: "/console", Handler: A.consoleHandler},
			"trace":      {Endpoint: "trace", Path: "/trace", Handler: A.traceHandler},
			"theme":      {Endpoint: "theme", Path: "/theme", Handler: A.themeHandler},
			"ping":       {Endpoint: "ping", Path: "/ping", Handler: A.pingHandler},
			"static":     {Endpoint: "static", Path: "/static/", StaticFolder: "static"},
		}}

	A.Blueprint.registerTo(A.mux, "")

	// TODO: read lang from config
	gotext.Configure("translations", "en", "admin")

	A.security = AddSecurity(A, nil)
	return A
}

type Admin struct {
	*BaseView
	views []View
	dbs   map[string]*gorm.DB

	debug             bool
	autoMigrate       bool
	trace             bool
	tracer            *Trace
	key               []byte
	sessionKey        string
	store             sessions.Store
	csrf              func(http.Handler) http.Handler
	mux               *http.ServeMux
	indexTemplateFile string
	theme             string
	security          *Security
}

func (A *Admin) Session(r *http.Request) *sessions.Session {
	// store.Options.SameSite = http.SameSiteStrictMode // TODO:
	sess, err := sessions.GetRegistry(r).Get(A.store, A.sessionKey)
	if err != nil {
		panic(err)
	}
	return sess
}

func (A *Admin) Register(b *Blueprint) {
	if err := A.Blueprint.AddChild(b); err != nil {
		log.Print(err)
		return
	}

	b.registerTo(A.mux, A.Blueprint.Path)
}

func (A *Admin) AddView(view View) View {
	view.setAdmin(A)

	if b := view.GetBlueprint(); b != nil {
		A.views = append(A.views, view)
		A.Register(b)

		A.addViewToMenu(view)
	}

	if mv, ok := view.(*ModelView); ok {
		if !slices.Contains(lo.Values(A.dbs), mv.db) {
			A.dbs[mv.Blueprint.Name] = mv.db
		}

		if A.trace {
			A.tracer.Trace(mv.db)
		}
	}
	return view
}

func (A *Admin) FindView(endpoint string) View {
	v, _ := lo.Find(A.views, func(v View) bool {
		return v.GetBlueprint().Endpoint == endpoint
	})
	return v
}

func (A *Admin) addViewToMenu(view View) {
	if menu := view.GetMenu(); menu != nil {
		// CAUTION: patch MenuItem.Path
		if menu.Path == "" {
			menu.Path, _ = A.Blueprint.GetUrl(view.GetBlueprint().Endpoint + ".index")
		}
		A.BaseView.Menu.AddMenu(menu, menu.Category)
	}
}
func (A *Admin) freeze() {
	db2ms := map[*gorm.DB][]any{}

	for _, v := range A.views {
		if mv, ok := v.(*ModelView); ok {
			db2ms[mv.db] = append(db2ms[mv.db], mv.Model.new())
			mv.freeze()
		}
	}

	// migrate all here, single table migration won't create many2many table
	if A.autoMigrate {
		for db, ms := range db2ms {
			db.AutoMigrate(ms...)
		}
	}
}
func (A *Admin) staticURL(filename, ver string) string {
	path, err := A.Blueprint.GetUrl(".static")
	if err != nil {
		panic(err)
	}

	if ver != "" {
		return path + filename + "?ver=" + ver
	}
	return path + filename
}

// Flask.url_for, `endpoint` like:
// admin.index
// model.create_view
// .create_view
func (A *Admin) UrlFor(model, endpoint string, args ...any) (string, error) {
	prefix := ""
	b := A.Blueprint
	if model != "" {
		cb, ok := A.Blueprint.Children[model]
		if !ok {
			return "", fmt.Errorf("model '%s' miss", model)
		}
		prefix = A.Blueprint.Path
		b = cb
	}

	res, err := b.GetUrl(endpoint, args...)
	if err != nil {
		return "", err
	}
	return prefix + res, nil
}

func (A *Admin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/admin/static/") ||
		strings.HasPrefix(r.URL.Path, "/.well-known/") {
		A.mux.ServeHTTP(w, r)
		return
	}

	// for http
	r = csrf.PlaintextHTTPRequest(r)

	// make sure session put in r.Context
	_ = sessions.GetRegistry(r)

	cw := NewCachedWriter(w)
	// csrf protect
	handlers.LoggingHandler(os.Stdout,
		A.csrf(A.mux)).ServeHTTP(cw, r)

	// save sesstion before flush
	if err := sessions.Save(r, cw); err != nil {
		panic(err)
	}

	cw.Flush()

	// trace
	if A.tracer != nil {
		defer A.tracer.CheckTrace(r)
	}
}

func (A *Admin) Run() {
	serv := http.Server{
		Addr:    ":3333",
		Handler: A}

	fmt.Println("\aRunning on http://127.0.0.1:3333/admin/")
	A.freeze()
	serv.ListenAndServe()
}

// template function
func (*Admin) marshal(v any) string {
	bs, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(bs)
}
func (*Admin) config(key string) bool {
	return false
}
func (*Admin) gettext(format string, a ...any) string {
	return gettext(format, a...)
}

// convince for outside of `Admin`
func gettext(format string, a ...any) string {
	return gotext.Get(format, a...)
}

var themes = []string{
	"cyborg", "cerulean", "default",
	// "solar", "superhero", "darkly", "slate", // night
	// "cosmo", "flatly", "journal", "litera",
	// "lumen", "lux", "materia", "minty", "united", "pulse",
	// "sandstone", "simplex", "sketchy", "spacelab", "yeti",
}

func (A *Admin) dict(others ...map[string]any) map[string]any {
	o := map[string]any{
		"debug":    A.debug,
		"security": A.security,
		"db":       len(A.dbs),
		"name":     A.Blueprint.Name,
		"url":      A.Blueprint.Path, // "/admin"
		// 'swatch' from flask-admin
		"swatch": A.theme,
		"menu":   A.BaseView.Menu,
		"config": config,
	}

	if len(others) > 0 {
		merge(o, others[0])
	}
	return o
}

func (A *Admin) indexHandler(w http.ResponseWriter, r *http.Request) {
	A.Render(w, r, A.indexTemplateFile, nil, A.dict())
}
func (A *Admin) pingHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("ping"))
}
func (A *Admin) debugHandler(w http.ResponseWriter, r *http.Request) {
	cv := r.Context().Value(csrf.PlaintextHTTPContextKey)
	if cv == nil {
		panic("not found PlaintextHTTPContextKey")
	}

	ReplyJson(w, 200, A.dict(map[string]any{
		"blueprint": A.Blueprint.dict(),
	}))
}
func (A *Admin) debugHtmlHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("content-type", ContentTypeUtf8Html)
	tx, err := template.New("debug").
		Option("missingkey=error").
		Funcs(A.funcs(template.FuncMap{
			"get_flashed_messages": func() []any { return A.Session(r).Flashes() },
		})).
		ParseFiles("templates/debug.tmpl")
	if err != nil {
		panic(err)
	}

	err = tx.Lookup("debug.tmpl").Execute(w, A.dict())
	if err != nil {
		w.Write([]byte(err.Error()))
	}
}

// serve admin.static
// url /admin/static/{} => local static/{}
// first way:
// fs := http.FileServer(http.Dir("static"))
// a.Mux.Handle("/admin/static/", http.StripPrefix("/admin/static/", fs))
//
// second way:
// a.Mux.HandleFunc("/admin/static/{path...}",
//
//	func(w http.ResponseWriter, r *http.Request) {
//	        path := r.PathValue("path")
//	        http.ServeFileFS(w, r, os.DirFS("static"), path)
//	})
// func (A *Admin) static_handle(w http.ResponseWriter, r *http.Request) {}

func (A *Admin) funcs(more template.FuncMap) template.FuncMap {
	res := merge(sprig.FuncMap(), Funcs)
	merge(res, template.FuncMap{
		"admin_static_url": A.staticURL, // used
		"marshal":          A.marshal,   // test
		"config":           A.config,    // used
		"gettext":          A.gettext,   //
		"get_url":          A.Blueprint.GetUrl,
		// escape safe
		"safehtml": func(s string) template.HTML { return template.HTML(s) },
		"safejs":   func(s string) template.JS { return template.JS(s) },
		"json": func(v any) (template.JS, error) {
			bs, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return template.JS(string(bs)), nil
		},
	})

	if more != nil {
		merge(res, more)
	}
	return res
}

func (A *Admin) SetIndexTemplateFile(nfn string) {
	A.indexTemplateFile = nfn
}

type wsWriter struct {
	*websocket.Conn
}

func (w *wsWriter) Write(p []byte) (n int, err error) {
	w.WriteMessage(websocket.TextMessage, p)
	return n, nil
}

var g *generator

func (A *Admin) generateHandler(w http.ResponseWriter, r *http.Request) {
	if websocket.IsWebSocketUpgrade(r) {
		upgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade failed", err)
			return
		}

		// ignore all input
		go conn.ReadMessage()
		go func() {
			g.Run(A, &wsWriter{conn})
			g = nil // cleanup
		}()
		return
	}

	if r.Method == http.MethodGet {
		// GET
		A.Render(w, r, "templates/generate.tmpl", nil, map[string]any{
			"gen":        nil,
			"csrf_field": csrf.TemplateField(r),
		})
	} else {
		// POST
		g = NewGenerator(r.FormValue("url"))
		g.Package = r.FormValue("package")
		A.Render(w, r, "templates/generate.tmpl", nil, map[string]any{
			"gen":        g,
			"csrf_field": csrf.TemplateField(r),
		})
	}
}
func (A *Admin) consoleHandler(w http.ResponseWriter, r *http.Request) {
	result := &Result{Query: DefaultQuery(), Rows: []*Row{}}
	var name string
	var sql string
	if r.Method == http.MethodPost {
		sql = r.FormValue("sql")
		name = r.FormValue("name")
		db, ok := A.dbs[name]
		if !ok {
			result.Error = fmt.Errorf("db %s not exists", name)
		} else {
			// CAUTION: non-checked sql, even drop table
			var rs []map[string]any
			tx := db.Raw(sql).Scan(&rs)
			// Raw/Scan not support offset/limit
			// TODO: only scan 20 records?

			result.Error = tx.Error
			result.Total = tx.RowsAffected
			// TODO: How to cast better?
			result.Rows = make([]*Row, len(rs))
			for i := 0; i < len(rs); i++ {
				result.Rows[i] = &Row{Map: rs[i]}
			}
		}
	}
	A.Render(w, r, "templates/console.tmpl", nil, map[string]any{
		"sql":        sql,
		"result":     result,
		"dbs":        lo.Keys(A.dbs),
		"name":       name,
		"csrf_field": csrf.TemplateField(r),
	})
}
func (A *Admin) traceHandler(w http.ResponseWriter, r *http.Request) {
	m := map[string]any{"entries": nil}
	if A.tracer != nil {
		m["entries"] = A.tracer.Entries()
	}

	A.Render(w, r, "templates/trace.tmpl", nil, m)
}

func (A *Admin) themeHandler(w http.ResponseWriter, r *http.Request) {
	nt := r.URL.Query().Get("name")
	A.theme = nt
	url := r.Header.Get("referer")
	http.Redirect(w, r, url, http.StatusFound)
}
