package gadm

import (
	"encoding/csv"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/fatih/camelcase"
	"github.com/go-playground/form/v4"
	"github.com/gorilla/csrf"
	"github.com/samber/lo"
	"github.com/spf13/cast"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type ModelView struct {
	*BaseView
	*Model
	db *gorm.DB

	// Permissions
	can_create       bool
	can_edit         bool
	can_delete       bool
	can_view_details bool
	can_export       bool

	// Customizations
	column_list          []string
	column_exclude_list  []string
	column_editable_list []string
	column_sortable_list []string
	column_descriptions  map[string]string

	table_prefix_html string

	// Pagination settings
	page_size         int
	can_set_page_size bool
	column_filters    []string

	column_searchable_list []string

	column_display_pk      bool
	column_display_actions bool

	lookupRefers map[string]*refer

	// form
	form_choices          map[string][]Choice
	form_columns          []string
	form_excluded_columns []string

	filters []Filter

	// <textarea row=5>
	textareaRow map[string]int
	// TODO: date-format="YYYY-MM-DD"

	// db relation settings
	joins      []queryArg
	innerJoins []queryArg
	preloads   []queryArg

	fsList []*Field
	fsNew  []*Field
	fsEdit []*Field

	gt *groupTempl
}

// for db.Joins(query string, args... any)
type queryArg struct {
	query string
	args  []any
}

// TODO: ensure m not ptr
// ep: endpoint for this view
func NewModelView(m any, db *gorm.DB, eps ...string) *ModelView {
	model := NewModel(m)

	mv := ModelView{
		BaseView:               NewView(Menu{Name: model.label()}),
		db:                     db,
		Model:                  model,
		can_create:             true,
		can_edit:               true,
		can_delete:             true,
		can_view_details:       true,
		can_export:             true, // false
		page_size:              20,
		can_set_page_size:      false,
		column_display_actions: true,
		//
		column_descriptions: map[string]string{},
	}
	mv.SetRoles("user")

	mv.Blueprint = &Blueprint{
		Name:     model.label(),
		Endpoint: firstOr(eps, model.endpoint()),
		Path:     "/" + firstOr(eps, model.endpoint()),
		Children: map[string]*Blueprint{
			// In flask-admin use `view.index`. Should use `view.index_view` in `gadmin`
			"index":        {Endpoint: "index", Path: "/", Handler: mv.indexHandler},
			"index_view":   {Endpoint: "index_view", Path: "/", Handler: mv.indexHandler},
			"create_view":  {Endpoint: "create_view", Path: "/new", Handler: mv.newHandler},
			"details_view": {Endpoint: "details_view", Path: "/details", Handler: mv.detailHandler},
			"ajax_update":  {Endpoint: "ajax_update", Path: "/ajax/update", Handler: mv.ajaxUpdate},
			"ajax_lookup":  {Endpoint: "ajax_lookup", Path: "/ajax/lookup", Handler: mv.ajaxLookup},
			"action":       {Endpoint: "action", Path: "/action", Handler: mv.actionHandler},
			"edit_view":    {Endpoint: "edit_view", Path: "/edit", Handler: mv.editHandler},
			"delete_view":  {Endpoint: "delete_view", Path: "/delete", Handler: mv.deleteHandler},
			// not .export_view
			"export": {Endpoint: "export", Path: "/export", Handler: mv.exportHandler},
			"debug":  {Endpoint: "debug", Path: "/debug", Handler: mv.debugHandler},
			// for json
			"list": {Endpoint: "list", Path: "/list", Handler: mv.listJson},
		},
	}

	mv.column_sortable_list = mv.sortableColumns()

	mv.gt = NewGroupTempl(
		"templates/base.tmpl",
		"templates/layout.tmpl",
		"templates/lib.tmpl",
		"templates/model_layout.tmpl",
		"templates/form.tmpl",
		"templates/model_row_actions.tmpl",
	)
	return &mv
}

func (V *ModelView) Joins(query string, args ...any) *ModelView {
	V.joins = append(V.joins, queryArg{query, args})
	return V
}
func (V *ModelView) InnerJoins(query string, args ...any) *ModelView {
	V.innerJoins = append(V.innerJoins, queryArg{query, args})
	return V
}
func (V *ModelView) Preloads(query string, args ...any) *ModelView {
	V.preloads = append(V.preloads, queryArg{query, args})
	return V
}

// Permissions
// Is model creation allowed
func (V *ModelView) SetCanCreate(v bool) *ModelView {
	V.can_create = v
	return V
}

// Is model editing allowed
func (V *ModelView) SetCanEdit(v bool) *ModelView {
	V.can_edit = v
	return V
}
func (V *ModelView) SetCanExport(v bool) *ModelView {
	V.can_export = v
	return V
}

// Collection of the model field names for the list view.
// If not set, will get them from the model.
func (V *ModelView) SetColumnList(vs ...string) *ModelView {
	V.column_list = vs
	return V
}

func (V *ModelView) SetColumnEditableList(vs ...string) *ModelView {
	V.column_editable_list = vs
	return V
}
func (V *ModelView) SetColumnDescriptions(m map[string]string) *ModelView {
	V.column_descriptions = m
	return V
}
func (V *ModelView) SetTablePrefixHtml(v string) *ModelView {
	V.table_prefix_html = v
	return V
}
func (V *ModelView) SetCanSetPageSize() *ModelView {
	V.can_set_page_size = true
	return V
}
func (V *ModelView) SetPageSize(v int) *ModelView {
	V.page_size = v
	return V
}
func (V *ModelView) SetColumnSearchableList(s ...string) *ModelView {
	V.column_searchable_list = s
	return V
}
func (V *ModelView) SetTextareaRow(rows map[string]int) *ModelView {
	V.textareaRow = rows
	return V
}
func (V *ModelView) SetColumnFilters(fs ...string) *ModelView {
	V.column_filters = fs
	return V
}

// form
func (V *ModelView) SetFormChoices(choices map[string][]Choice) *ModelView {
	V.form_choices = choices
	return V
}
func (V *ModelView) SetFormColumns(columns ...string) *ModelView {
	V.form_columns = columns
	return V
}
func (V *ModelView) SetFormExcludedColumns(columns ...string) *ModelView {
	V.form_excluded_columns = columns
	return V
}

type refer struct {
	fields []string
	model  *Model
}

func astoss(as []any) string {
	return strings.Join(lo.Map(as, func(a any, _ int) string { return cast.ToString(a) }), ",")
}

// refer.fields like query
// Return slice of [id, "f1, f2"]
func (rf *refer) Lookup(db *gorm.DB, query string, offset, limit int) [][]any {
	ndb := db.Limit(limit).Offset(offset)

	pks := []string{}
	fns := lo.Map(lo.Filter(rf.model.Fields, func(f *Field, _ int) bool {
		if f.PrimaryKey {
			pks = append(pks, f.DBName)
			return true
		}
		if slices.Contains(rf.fields, f.DBName) {
			return true
		}
		return false
	}), func(f *Field, _ int) string {
		return f.DBName
	})

	ndb = ndb.Table(rf.model.schema.Table).
		Select(fns)

	for i, f := range rf.fields {
		if i == 0 {
			ndb = ndb.Where(fmt.Sprintf("%s like ?", rf.fields[0]), like(query))
		} else {
			ndb = ndb.Or(fmt.Sprintf("%s like ?", f), like(query))
		}
	}
	var ms []map[string]any
	if tx := ndb.Find(&ms); tx.Error == nil {
		return lo.Map(ms, func(m map[string]any, _ int) []any {
			var id any
			ids := []any{}
			for _, pk := range pks {
				ids = append(ids, m[pk])
			}
			if len(ids) > 1 {
				id = astoss(ids)
			} else {
				id = ids[0]
			}

			vs := []any{}
			for _, f := range rf.fields {
				vs = append(vs, m[f])
			}
			return []any{id, astoss(vs)}
		})
	}
	return [][]any{}
}

// user: [first_name, last_name]
// lookup table user's first_name, last_name
func (V *ModelView) AddLookupRefer(a any, fields ...string) *ModelView {
	if reflect.ValueOf(a).Kind() != reflect.Struct {
		log.Println("wrong refer type, 'a' should be Struct")
	}

	if V.lookupRefers == nil {
		V.lookupRefers = map[string]*refer{}
	}
	rf := &refer{model: NewModel(a), fields: fields}
	V.lookupRefers[rf.model.name()] = rf
	return V
}

func (V *ModelView) transform(fs []*schema.Field) []*Field {
	return lo.Map(fs, func(f *schema.Field, _ int) *Field {
		return &Field{
			Field:       f,
			Label:       strings.Join(camelcase.Split(f.Name), " "),
			Choices:     V.form_choices[f.DBName],
			Description: emptyOr(V.column_descriptions[f.DBName], f.Comment),
			TextAreaRow: V.textareaRow[f.DBName],
			Readonly:    !V.can_edit,
			Sortable:    slices.Contains(V.column_sortable_list, f.DBName),
		}
	})
}

func (V *ModelView) freeze() {
	fs := V.transform(V.schema.Fields)

	V.fsList = lo.Filter(fs, func(field *Field, _ int) bool {
		// exclude return false
		if slices.Contains(V.column_exclude_list, field.DBName) {
			return false
		}

		// include, return true
		if slices.Contains(V.column_list, field.DBName) {
			return true
		}

		// keep, but hidden
		if field.PrimaryKey {
			return true
		}
		return len(V.column_list) == 0
	})
	if !V.column_display_pk {
		V.fsList = clone(V.fsList)
		for _, f := range V.fsList {
			if f.PrimaryKey {
				f.Hidden = true
			}
		}
	}

	V.fsNew = lo.Filter(fs, func(f *Field, _ int) bool {
		// exclude, return false
		if slices.Contains(V.form_excluded_columns, f.DBName) {
			return false
		}
		// include, return true
		if slices.Contains(V.form_columns, f.DBName) {
			return true
		}
		return !f.PrimaryKey
	})

	// need clone: change Readonly
	V.fsEdit = clone(lo.Filter(fs, func(f *Field, _ int) bool {
		// exclude, return false
		if slices.Contains(V.form_excluded_columns, f.DBName) {
			return false
		}
		// include, return true
		if slices.Contains(V.form_columns, f.DBName) {
			return true
		}
		// inline editable should ajax editable
		if slices.Contains(V.column_editable_list, f.DBName) {
			return true
		}
		if len(V.form_columns) > 0 {
			return f.PrimaryKey
		}
		return true
	}))
	for _, f := range V.fsEdit {
		if f.PrimaryKey {
			f.Readonly = true
		}
	}

	V.filters = []Filter{}
	for _, name := range V.column_filters {
		if f, ok := lo.Find(fs, func(f *Field) bool {
			return f.DBName == name
		}); ok {
			V.filters = append(V.filters, V.filtersOf(f)...)
		}
	}
	for i := 0; i < len(V.filters); i++ {
		V.filters[i].Index = i
		V.filters[i].Arg = cast.ToString(i)
	}
}
func (V *ModelView) filtersOf(f *Field) []Filter {
	switch f.DataType {
	case schema.Bool:
		return []Filter{
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "equal", Options: [][]string{{"1", "Yes"}, {"0", "No"}}},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not equal", Options: [][]string{{"1", "Yes"}, {"0", "No"}}},
		}
	case schema.Int:
		return []Filter{
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "equal"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not equal"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "greater"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "smaller"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "empty", Options: [][]string{{"1", "Yes"}, {"0", "No"}}},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "in list", WidgetType: ptr("select2-tags")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not in list", WidgetType: ptr("select2-tags")},
		}
	case schema.String:
		return []Filter{
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "like"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not like"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "equal"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not equal"},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "empty", Options: [][]string{{"1", "Yes"}, {"0", "No"}}},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "in list", WidgetType: ptr("select2-tags")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not in list", WidgetType: ptr("select2-tags")},
		}
	case schema.Time:
		return []Filter{
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "equal", WidgetType: ptr("datetimepicker")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not equal", WidgetType: ptr("datetimepicker")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "greater", WidgetType: ptr("datetimepicker")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "smaller", WidgetType: ptr("datetimepicker")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "between", WidgetType: ptr("datetimerangepicker")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "not between", WidgetType: ptr("datetimerangepicker")},
			{Field: f, Label: f.Label, DBName: f.DBName, Operation: "empty", Options: [][]string{{"1", "Yes"}, {"0", "No"}}},
		}
	}
	return nil
}

func toGroup(fs []Filter) map[string][]Filter {
	g := map[string][]Filter{}
	for _, f := range fs {
		g[f.Label] = append(g[f.Label], f)
	}
	return g
}

func (V *ModelView) dict(r *http.Request, others ...map[string]any) map[string]any {
	o := V.BaseView.dict(r, map[string]any{
		"table_prefix_html": V.table_prefix_html,
		"editable_columns":  true,
		"can_create":        V.can_create,
		"can_edit":          V.can_edit,
		"can_export":        V.can_export,
		"can_view_details":  V.can_view_details,
		"can_delete":        V.can_delete,
		"export_types":      []string{"csv", "xls"},
		// TODO: modal for edit/create/details
		"edit_modal":    false,
		"create_modal":  false,
		"details_modal": false,
		"is_modal":      false,
		"form_opts": map[string]any{
			"widget_args": nil,
			"form_rules":  []any{},
		},
		"return_url": must(V.Blueprint.GetUrl(".index_view")),
	})

	if len(others) > 0 {
		merge(o, others[0])
	}
	return o
}

// Because `default_page_size`, should place here, not query.go
func (V *ModelView) queryFrom(r *http.Request) *Query {
	base, _ := V.Blueprint.GetUrl(".index")
	q := Query{default_page_size: V.page_size, PageSize: V.page_size,
		base: base}
	r.ParseForm()
	uv := r.Form

	form.NewDecoder().Decode(&q, uv)
	for k, v := range uv {
		if lo.IndexOf([]string{"page", "page_size", "sort", "desc", "search"}, k) != -1 {
			continue
		}
		if strings.HasPrefix(k, "flt") {
			f := V.inputFilter(k, v[0])
			if f != nil {
				q.filters = append(q.filters, f)
			}
		} else {
			q.args = append(q.args, k, v[0])
		}
	}
	return &q
}

// flt0_35=2024-10-28&flt2_27=Harry&flt3_0=1
func (V *ModelView) inputFilter(k, v string) *InputFilter {
	arr := lo.Map(strings.Split(k[2:], "_"), func(s string, _ int) int {
		return cast.ToInt(s)
	})
	if arr[1] < len(V.filters) {
		f := V.filters[arr[1]]
		return &InputFilter{Label: f.Label, Index: arr[1], Query: v}
	}
	return nil
}

func (V *ModelView) get_column_index(name string) int {
	if _, i, ok := lo.FindIndexOf(V.fsList, func(f *Field) bool {
		return f.DBName == name
	}); ok {
		return i
	}
	return -1
}

func (V *ModelView) column_name(i int) string {
	if i < len(V.fsList) {
		return V.fsList[i].DBName
	}
	return ""
}

func (V *ModelView) is_editable(name string) bool {
	if !V.can_edit {
		return false
	}
	_, ok := lo.Find(V.column_editable_list, func(i string) bool {
		return i == name
	})
	return ok
}

func (V *ModelView) list_row_actions(r *http.Request) []Action {
	actions := []Action{}
	if V.can_view_details {
		actions = append(actions, Action{
			Name:  "view",
			Title: gettext("View Record"),
		})
	}
	if V.can_edit {
		actions = append(actions, Action{
			Name:  "edit",
			Title: gettext("Edit Record"),
		})
	}
	if V.can_delete {
		actions = append(actions, Action{
			Name:         "delete",
			Title:        gettext("Delete Record"),
			Confirmation: gettext("Are you sure you want to delete selected records?"),
			CSRFToken:    csrf.Token(r),
		})
	}
	return actions
}

func (V *ModelView) debugHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("a") == "1" {
		V.AddFlash(r, FlashSuccess(`Record was successfully deleted.
1 records were successfully deleted.`))
		V.redirect(w, r, "/admin/company")
		return
	}
	w.Header().Set("foo", "bar")

	V.Render(w, r, "debug.tmpl", nil, map[string]any{
		"query":     V.queryFrom(r),
		"menu":      V.Menu.dict(r.URL.Path, CurrentRoles(r)),
		"blueprint": V.Blueprint,
	})
}
func (V *ModelView) indexHandler(w http.ResponseWriter, r *http.Request) {
	q := V.queryFrom(r)

	result := V.list(q)
	result.Fields = V.fsList

	V.Render(w, r, "model_list.tmpl", template.FuncMap{
		"is_sortable": func(name string) bool {
			return slices.Contains(V.column_sortable_list, name)
		},
		"sort_url": func(name string, invert ...bool) string {
			q := *q // simply copy
			q.Sort = cast.ToString(V.get_column_index(name))
			q.Desc = firstOr(invert)
			return must(V.Blueprint.GetUrl(".index_view", queryToPairs(q.toValues())...))
		},
		"column_descriptions": func(name string) string {
			if desc, ok := V.column_descriptions[name]; ok {
				return desc
			}
			return V.find(name).Description
		},
	}, map[string]any{
		"count":             len(result.Rows),
		"page":              q.Page,
		"num_pages":         result.NumPages(),
		"page_size":         q.PageSize,
		"default_page_size": q.default_page_size,
		"page_size_url": func(page_size int) string {
			uv := q.toValues()
			uv.Set("page_size", cast.ToString(page_size))
			return must(V.Blueprint.GetUrl(".index_view", queryToPairs(uv)...))
		},
		"can_set_page_size":        V.can_set_page_size,
		"data":                     result.Rows, // TODO: remove
		"result":                   result,
		"request":                  rd(r),
		"column_display_pk":        V.column_display_pk,
		"column_display_actions":   V.column_display_actions,
		"column_extra_row_actions": nil,
		"list_row_actions":         V.list_row_actions(r),
		"actions": []Action{{Name: "delete", Title: "Delete",
			CSRFToken: csrf.Token(r),
			URL:       must(V.Blueprint.GetUrl(".action")),
			ReturnURL: must(V.Blueprint.GetUrl(".index_view"))}},
		"actions_confirmation": map[string]string{"delete": "Are you sure you want to delete selected records?"},
		"list_columns":         V.fsList,
		"sort":                 q.Sort,
		// not func, return current sort field name
		// in template, `sort url` is: ?sort={index}
		// transform `index` to `column name`
		"sort_column": func() string {
			if q.Sort != "" {
				idx := cast.ToInt(q.Sort)
				return V.column_name(idx)
			}
			return ""
		}(),
		"sort_desc": q.Desc,

		// search
		"search":                 q.Search,
		"column_searchable_list": V.column_searchable_list,
		"search_placeholder":     strings.Join(V.column_searchable_list, ","),

		// filter
		"filters":        len(V.column_filters) > 0,
		"filter_groups":  toGroup(V.filters),
		"active_filters": activeFilter(q.filters), // [[27, "Title", "part"]]
		"clear_search_url": func() string {
			qc := *q
			qc.Search = ""
			return must(V.Blueprint.GetUrl(".index_view", queryToPairs(qc.toValues())...))
		}(),
	})
}
func (V *ModelView) listJson(w http.ResponseWriter, r *http.Request) {
	q := V.queryFrom(r)

	res := V.list(q)
	res.Fields = V.fsList
	if res.Error != nil {
		ReplyJson(w, 200, map[string]any{"error": res.Error})
		return
	}
	ReplyJson(w, 200, map[string]any{"total": res.Total, "data": res.Rows})
}

func (V *ModelView) newHandler(w http.ResponseWriter, r *http.Request) {
	q := V.queryFrom(r)
	if !V.can_create {
		V.redirect(w, r, q.Get("url"))
		return
	}

	if r.Method == http.MethodPost {
		// trigger ParseMultipartForm
		continue_editing := r.PostFormValue("_continue_editing")

		one := V.intoRow(r.PostForm, V.fsNew)
		err := V.create(one)
		if err != nil {
			V.AddFlash(r, FlashError(err))
		} else {
			V.AddFlash(r, FlashInfo(gettext("Record was successfully created.")))
		}

		if continue_editing != "" {
			rowid := one.GetPkValue()
			V.redirect(w, r, must(V.Blueprint.GetUrl(".edit_view", "id", rowid)))
			return
		}
		if r.PostFormValue("_add_another") != "" {
			V.redirect(w, r, must(V.Blueprint.GetUrl(".create_view")))
			return
		}

		V.redirect(w, r, q.Get("url"))
		return
	}

	// GET
	V.Render(w, r, "model_create.tmpl", nil, map[string]any{
		"request":    rd(r),
		"form":       NewForm(V.fsNew, nil, csrf.Token(r)),
		"cancel_url": must(V.Blueprint.GetUrl(".index_view")),
		"form_opts": map[string]any{
			"widget_args": nil, "form_rules": nil,
		},
		"action": nil,
	})
}

func (V *ModelView) redirect(w http.ResponseWriter, r *http.Request, urls ...string) {
	path := firstOr(urls)
	if path == "" {
		path = r.URL.Query().Get("url")
	}
	if path == "" {
		path = must(V.Blueprint.GetUrl(".index_view"))
	}
	http.Redirect(w, r, path, http.StatusFound)
}

func (V *ModelView) editHandler(w http.ResponseWriter, r *http.Request) {
	q := V.queryFrom(r)
	rowid := q.Get("id")

	if !V.can_edit || rowid == "" {
		V.redirect(w, r, q.Get("url"))
		return
	}

	row, err := V.getOne(rowid)
	if err != nil {
		V.AddFlash(r, FlashInfo(gettext("Record does not exist.")))
		V.redirect(w, r, q.Get("url"))
		return
	}
	if r.Method == http.MethodPost {
		one := V.intoRow(r.PostForm, V.fsEdit)
		if V.update(rowid, one) != nil {
			V.AddFlash(r, FlashDanger(gettext("Record does not exist.")))
		}

		if r.PostFormValue("_add_another") != "" {
			V.redirect(w, r, must(V.Blueprint.GetUrl(".create_view")))
			return
		} else if r.PostFormValue("_continue_editing") != "" {
			// do nothing next Render
		} else {
			// Save only, redirect to url
			V.redirect(w, r)
			return
		}
	}

	V.Render(w, r, "model_edit.tmpl", nil, map[string]any{
		"row":     row,
		"form":    NewForm(V.fsEdit, row, csrf.Token(r)),
		"request": rd(r),
	})
}

// Model().Where(pk field = pk value).Delete()
func (V *ModelView) deleteHandler(w http.ResponseWriter, r *http.Request) {
	q := V.queryFrom(r)
	rowid := q.Get("id")
	if !V.can_delete || rowid == "" {
		V.redirect(w, r)
		return
	}

	err := V.deleteOne(rowid)
	if err != nil {
		V.AddFlash(r, Flash(gettext("Failed to delete record. %s", err), "error"))
	} else {
		V.AddFlash(r, FlashSuccess(gettext(`Record was successfully deleted.
1 records were successfully deleted.`)))
	}
	V.redirect(w, r)
}

// Model().Where(pk field = pk value).First()
func (V *ModelView) detailHandler(w http.ResponseWriter, r *http.Request) {
	q := V.queryFrom(r)
	rowid := q.Get("id")

	if !V.can_view_details || rowid == "" {
		V.redirect(w, r)
		return
	}

	row, err := V.getOne(rowid)
	if err != nil {
		V.AddFlash(r, FlashDanger(gettext("Record does not exist.")))

		V.redirect(w, r)
		return
	}

	V.Render(w, r, "model_details.tmpl", nil, map[string]any{
		"row":             row,
		"details_columns": V.Fields, // show all fields
		"request":         rd(r),
	})
}

// list_form_pk=a1d13310-7c10-48d5-b63b-3485995ad6a4&currency=USD
// Record was successfully saved.
// Model().Where().Update(currency=USD)
func (V *ModelView) ajaxUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}

	if len(V.column_editable_list) == 0 {
		w.WriteHeader(404)
		return
	}

	// form
	r.ParseForm()
	rowid := r.Form.Get("list_form_pk")

	row := V.intoRow(r.Form, V.fsEdit)

	// TODO: validate

	// update_model
	if err := V.update(rowid, row); err != nil {
		w.WriteHeader(500)
		w.Write([]byte(V.admin.gettext("Failed to update record. %s", err)))
		return
	}
	w.Write([]byte(V.admin.gettext("Record was successfully saved.")))
}

// /admin/employee/ajax/lookup?name=employee&query=l&offset=0&limit=10&_=1763527783488
func (V *ModelView) ajaxLookup(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if rf, ok := V.lookupRefers[name]; ok {
		query := r.FormValue("query")
		offset := cast.ToInt(r.FormValue("offset"))
		limit := cast.ToInt(r.FormValue("limit"))
		rs := rf.Lookup(V.db, query, offset, limit)
		ReplyJson(w, 200, rs)
	} else {
		ReplyJson(w, 200, []any{})
	}
}

// action=delete, rowid=[], url
func (V *ModelView) actionHandler(w http.ResponseWriter, r *http.Request) {
	action := r.FormValue("action")
	rowid := r.Form["rowid"]
	if !V.can_delete || len(rowid) == 0 {
		V.redirect(w, r)
		return
	}

	if action == "delete" {
		tx := V.deleteBatch(rowid)
		if tx.Error == nil {
			V.AddFlash(r, FlashSuccess(fmt.Sprintf("delete %d success", tx.RowsAffected)))
		} else {
			V.AddFlash(r, FlashError(tx.Error))
		}
	}
	V.redirect(w, r)
}
func (V *ModelView) exportHandler(w http.ResponseWriter, r *http.Request) {
	q := V.queryFrom(r)
	result := V.list(q)
	if result.Error != nil {
		panic(result.Error)
	}

	fn := fmt.Sprintf("attachment;filename=%s-%s.csv", V.name(),
		time.Now().Format(time.DateOnly))
	w.Header().Add("content-disposition", fn)
	w.Header().Add("content-type", "text/csv")

	cw := csv.NewWriter(w)

	// header
	header := lo.Map(result.Fields, func(f *Field, _ int) string {
		return f.DBName
	})
	cw.Write(header)

	for _, row := range result.Rows {
		line := lo.Map(row.Fields, func(f *Field, _ int) string {
			return f.Display()
		})
		cw.Write(line)
	}

	cw.Flush()
}

// request to dict, like flask.request
func rd(r *http.Request) map[string]any {
	return map[string]any{
		"method": r.Method,
		"url":    r.URL.String(),
		"args":   r.URL.Query(),
	}
}

// Generate inline edit form in list view
func (V *ModelView) inline_form(token string) func(field *Field, row *Row) template.HTML {
	return func(field *Field, row *Row) template.HTML {
		return InlineEdit(V.gt, token, V.Model, field, row)
	}
}
func (V *ModelView) delete_form() *modelForm {
	return &modelForm{Fields: []*Field{}}
}

func (V *ModelView) Render(w http.ResponseWriter, r *http.Request, name string, funcs template.FuncMap, data map[string]any) {
	fm := merge(template.FuncMap{
		"return_url": func() (string, error) {
			return V.Blueprint.GetUrl(".index_view")
		},
		// TODO: move into BaseView
		"get_flashed_messages": func() []any {
			return V.admin.Session(r).Flashes()
		},
		"get_url": func(endpoint string, args ...any) (string, error) {
			return V.Blueprint.GetUrl(endpoint, args...)
		},
		"csrf_token":  func() string { return csrf.Token(r) },
		"list_form":   V.inline_form(csrf.Token(r)),
		"delete_form": V.delete_form,
		"is_editable": V.is_editable,
	}, funcs)

	if err := V.gt.Render(w, "templates/"+name, V.admin.funcs(fm), V.dict(r, data)); err != nil {
		log.Printf("render failed: %s", err)
	}
}

// Parse form into map[string]any, only fields in current model
func (V *ModelView) intoRow(uv url.Values, fields []*Field) *Row {
	row := NewRow(fields, V.new())

	for _, f := range fields {
		if !uv.Has(f.DBName) {
			continue
		}

		var v any
		v = uv.Get(f.DBName)

		if len(f.Choices) > 0 {
			// fix field_select2 formerly None(in python) to null
			// TODO: fix in form.tmpl
			if v == "__None" {
				continue // ignore
			}
		}

		switch f.DataType {
		case schema.Bool:
			switch v {
			case "false":
				v = false
			case "true":
				v = true
			default:
				log.Printf("not expected bool %v\n", v)
				v = nil
			}
		case schema.Int:
			v = cast.ToInt(v)
		case schema.Uint:
			v = cast.ToUint(v)
		case schema.Float:
			v = cast.ToFloat64(v)
		case schema.Time:
			if v == "" {
				continue // ignore
			}
		case schema.String:
			if !f.NotNull && v == "" {
				continue
			}
		}

		row.Set(f, v)
	}
	return row
}

func (V *ModelView) applyJoins(db *gorm.DB) *gorm.DB {
	for _, q := range V.joins {
		db = db.Joins(q.query, q.args...)
	}
	for _, q := range V.innerJoins {
		db = db.InnerJoins(q.query, q.args...)
	}
	for _, q := range V.preloads {
		db = db.Preload(q.query, q.args...)
	}
	return db
}

func (V *ModelView) applyQuery(db *gorm.DB, q *Query, count_only bool) *gorm.DB {
	ndb := db
	limit := lo.Ternary(q.PageSize != 0, q.PageSize, q.default_page_size)
	if !count_only {
		ndb = ndb.Limit(limit)

		if q.Page > 0 {
			ndb = ndb.Offset(limit * q.Page)
		}
	}

	if q.Sort != "" {
		idx := cast.ToInt(q.Sort)
		column_name := V.column_name(idx)

		ndb = ndb.Order(clause.OrderByColumn{
			Column: clause.Column{Name: column_name},
			Desc:   q.Desc,
		})
	}

	// filter
	for _, inf := range q.filters {
		filter := V.filters[inf.Index]
		ndb = filter.Apply(ndb, inf.Query)
	}
	// search
	if q.Search != "" && len(V.column_searchable_list) > 0 {
		for i, c := range V.column_searchable_list {
			if i == 0 {
				ndb = ndb.Where(c+" like ?", like(q.Search))
			} else {
				ndb = ndb.Or(c+" like ?", like(q.Search))
			}
		}
	}
	return ndb
}

func (V *ModelView) list(q *Query) *Result {
	res := Result{Query: q}

	var total int64
	if err := V.applyQuery(V.db, q, true).
		Model(V.Model.new()).
		Count(&total).Error; err != nil {
		res.Error = err
		return &res
	}
	res.Total = total

	ptr := V.newSlice()
	db := V.applyQuery(V.db, q, false)
	if err := V.applyJoins(db).
		Find(ptr.Interface()).Error; err != nil {
		res.Error = err
		return &res
	}

	len := ptr.Elem().Len()

	res.Rows = make([]*Row, len)
	for i := 0; i < len; i++ {
		o := ptr.Elem().Index(i).Interface()
		res.Rows[i] = NewRow(V.fsList, o)
	}
	return &res
}

func (V *ModelView) getOne(rowid string) (*Row, error) {
	ptr := V.Model.new()
	db := V.applyJoins(V.db)
	if err := db.Where(V.where(rowid)).First(ptr).Error; err != nil {
		return nil, err
	}
	return NewRow(V.fsList, ptr), nil
}

func (V *ModelView) update(rowid string, row *Row) error {
	ptr := V.Model.new()
	if rc := V.db.Model(ptr).
		Where(V.where(rowid)).
		Updates(row.Map); rc.Error != nil || rc.RowsAffected != 1 {
		return rc.Error
	}
	return nil
}

func (V *ModelView) deleteOne(rowid string) error {
	ptr := V.Model.new()
	rc := V.db.Delete(ptr, V.where(rowid))
	return rc.Error
}
func (V *ModelView) deleteBatch(rowid []string) *gorm.DB {
	ptr := V.Model.new()
	ndb := V.db.Model(ptr)
	for i, id := range rowid {
		if i == 0 {
			ndb = ndb.Where(V.where(id))
		} else {
			ndb = ndb.Or(V.where(id))
		}
	}
	return ndb.Delete(ptr)
}

// row -> Model().Create() RETURNING *
func (V *ModelView) create(row *Row) error {
	ptr := V.Model.new()

	if rc := V.db.Model(ptr).
		Clauses(clause.Returning{}). // RETURNING *
		Create(row.Map); rc.Error != nil || rc.RowsAffected != 1 {
		return rc.Error
	}
	return nil
}
