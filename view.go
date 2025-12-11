package gadm

import (
	"encoding/gob"
	"html/template"
	"net/http"
	"strings"

	"github.com/gorilla/csrf"
	"github.com/samber/lo"
	"github.com/spf13/cast"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type View interface {
	// Add custom handler, eg: /admin/{endpoint}/{path}
	Expose(path string, h http.HandlerFunc)

	GetBlueprint() *Blueprint
	GetMenu() *Menu

	Render(http.ResponseWriter, *http.Request, string, template.FuncMap, map[string]any)

	setAdmin(*Admin)
}

type BaseView struct {
	Blueprint *Blueprint
	Menu      Menu
	admin     *Admin
	gt        *groupTempl
}

func NewView(menu Menu) *BaseView {
	return &BaseView{Blueprint: &Blueprint{Path: menu.Path},
		Menu: menu,
		gt: NewGroupTempl("templates/base.tmpl",
			"templates/layout.tmpl",
			"templates/lib.tmpl", // move to ModelView
			"templates/base.tmpl"),
	}
}

// Expose "/test" create Blueprint{Endpoint: "test", Path: "/test"}
func (V *BaseView) Expose(path string, h http.HandlerFunc) {
	// default `endpoint`, would be `.test`
	ep := strings.ToLower(strings.ReplaceAll(path, "/", ""))

	V.Blueprint.AddChild(
		&Blueprint{Endpoint: ep, Path: path, Handler: h},
	)
}

func (V *BaseView) GetBlueprint() *Blueprint { return V.Blueprint }
func (V *BaseView) GetMenu() *Menu           { return &V.Menu }

func (V *BaseView) Render(w http.ResponseWriter, r *http.Request, fn string, funcs template.FuncMap, data map[string]any) {
	fm := V.admin.funcs(funcs)
	fm["get_flashed_messages"] = func() []any {
		return V.admin.Session(r).Flashes()
	}
	fm["csrf_token"] = func() string { return csrf.Token(r) }

	if err := V.gt.Render(w, fn, fm, V.dict(r, data)); err != nil {
		panic(err)
	}
}

func (V *BaseView) setAdmin(admin *Admin) { V.admin = admin }
func (V *BaseView) SetRoles(rs ...string) *BaseView {
	V.Menu.Roles = rs
	return V
}

// category: success, danger, error, info
func (V *BaseView) AddFlash(r *http.Request, flash flash) {
	V.admin.Session(r).AddFlash(flash)
}

func (V *BaseView) dict(r *http.Request, others ...map[string]any) map[string]any {
	o := map[string]any{
		"path":               r.URL.Path,
		"name":               V.Menu.Name,
		"extra_css":          []string{},
		"extra_js":           []string{}, // "a.js", "b.js"}
		"admin":              V.admin.dict(r),
		"admin_fluid_layout": true,
		"current_user":       CurrentUser(r),
		"editable_columns":   nil,
	}

	if len(others) > 0 {
		merge(o, others[0])
	}
	return o
}

func parseTemplate(name string, funcs template.FuncMap, fn ...string) *template.Template {
	return template.Must(template.New(name).
		Option("missingkey=error").
		Funcs(funcs).
		ParseFiles(fn...))
}

func init() {
	gob.Register(flash{})
}

type flash struct {
	Data     string
	Category string
}

// flash category: success(green), info(blue), danger(red)
func Flash(data, category string) flash { return flash{data, category} }
func FlashSuccess(data string) flash    { return flash{data, "success"} }
func FlashInfo(data string) flash       { return flash{data, "info"} }
func FlashError(err error) flash        { return flash{err.Error(), "danger"} }
func FlashDanger(data string) flash     { return flash{data, "danger"} }

type Action struct {
	Name         string
	Title        string // TODO: Label
	Desc         string
	Confirmation string
	URL          string // form action
	ReturnURL    string
	CSRFToken    string
}

type Filter struct {
	Label      string     `json:"-"`
	Arg        string     `json:"arg"`
	Index      int        `json:"index"`     // 0-based
	Operation  string     `json:"operation"` // equal, contains
	Options    [][]string `json:"options"`
	WidgetType *string    `json:"type"` // select2-tags/datetimepicker/null
	DBName     string     `json:"-"`
	Field      *Field     `json:"-"`
}

func (f *Filter) Apply(db *gorm.DB, q string) *gorm.DB {
	switch f.Operation {
	case "empty":
		if q == "1" {
			db = db.Where(f.DBName + " IS NULL")
		} else {
			db = db.Where(f.DBName + " IS NOT NULL")
		}
	case "like":
		db = db.Where(f.DBName+" LIKE ?", like(q))
	case "not like":
		db = db.Where(f.DBName+" NOT LIKE ?", like(q))
	case "equal":
		if f.Field.DataType == schema.Bool {
			db = db.Where(f.DBName+" = ?", f.qbool(q))
		} else {
			db = db.Where(f.DBName+" = ?", q)
		}
	case "not equal":
		if f.Field.DataType == schema.Bool {
			db = db.Where(f.DBName+" <> ?", f.qbool(q))
		} else {
			db = db.Where(f.DBName+" <> ?", q)
		}
	case "greater":
		db = db.Where(f.DBName+" > ?", q)
	case "smaller":
		db = db.Where(f.DBName+" < ?", q)
	case "in list":
		db = db.Where(f.DBName+" IN ?", f.qlist(q))
	case "not in list":
		db = db.Where(f.DBName+" NOT IN ?", f.qlist(q))
	case "between":
		// 2025-11-19 00:00:00 to 2025-11-21 23:59:59
		ts := f.qbetween(q)
		db = db.Where(f.DBName+" BETWEEN ? AND ?", ts[0], ts[1])
	case "not between":
		ts := f.qbetween(q)
		db = db.Where(f.DBName+" NOT BETWEEN ? AND ?", ts[0], ts[1])
	}
	return db
}
func (f *Filter) qbetween(q string) []string {
	return strings.Split(q, " to ")
}
func (f *Filter) qbool(q string) bool {
	return q == "1"
}

func (f *Filter) qlist(q string) any {
	arr := strings.Split(q, ",")
	switch f.Field.DataType {
	case schema.String:
		return arr
	case schema.Int:
		return lo.Map(arr, func(s string, _ int) int { return cast.ToInt(s) })
	case schema.Uint:
		return lo.Map(arr, func(s string, _ int) uint { return cast.ToUint(s) })
	case schema.Float:
		return lo.Map(arr, func(s string, _ int) float64 { return cast.ToFloat64(s) })
	}
	return arr
}

type InputFilter struct {
	Label string
	Index int
	Query string
}

// [[27, "Title", "part"]]
func (a *InputFilter) toJson() any {
	return []any{a.Index, a.Label, a.Query}
}

// [[27, "Title", "part"]]
func activeFilter(inf []*InputFilter) any {
	arr := []any{}
	for _, f := range inf {
		arr = append(arr, f.toJson())
	}
	return arr
}
