package gadm

import (
	"fmt"
	"gadm/examples/sqla"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gopkg.in/guregu/null.v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

func views(db *gorm.DB) []*ModelView {
	return []*ModelView{
		NewModelView(sqla.AllTyped{}, db),
		NewModelView(sqla.Company{}, db),
		NewModelView(sqla.Employee{}, db),
		NewModelView(sqla.CreditCard{}, db),
		NewModelView(sqla.Holder{}, db),
		NewModelView(sqla.Address{}, db),
		NewModelView(sqla.Account{}, db),
		NewModelView(sqla.Language{}, db),
		NewModelView(sqla.Student{}, db),
		NewModelView(sqla.Toy{}, db),
		NewModelView(sqla.Dog{}, db),
	}
}

func TestModel(t *testing.T) {
	is := assert.New(t)

	// weird nil
	e := sqla.Employee{}
	is.Nil(e.Company)
	is.Nil(any(e.Company))
	str, ok := any(e.Company).(fmt.Stringer)
	is.True(ok)
	is.True(str != nil) // weird
	is.Nil(str)
	is.True(isNil(str))

	// field is struct
	re := NewRow(NewModel(sqla.Employee{}).Fields, &sqla.Employee{})
	fve := re.Fields[3]
	is.True(fve.IsStruct())
	is.Equal("Company", fve.Display())

	// field is slice
	rdog := NewRow(NewModel(sqla.Dog{}).Fields, &sqla.Dog{})
	fvslice := rdog.Fields[2]
	is.True(fvslice.IsSlice())
	is.False(fvslice.IsStruct())
	is.Equal("toys", fvslice.Endpoint())
	is.Equal("Toys", fvslice.Display())
	is.Equal(reflect.Slice, reflect.ValueOf(fvslice.Value).Kind())

	m := NewModel(sqla.AllTyped{})
	if true {
		is.Equal("all_typed", m.name())
		is.Equal("All Typed", m.label())
		is.Equal("alltyped", m.endpoint())

		is.Equal("ID", m.Fields[0].Label)
		is.Equal("id", m.Fields[0].DBName)
		is.Equal("ID", m.Fields[0].Name)
		is.Equal("Email", m.Fields[2].Label)
		is.Equal("Activated At", m.Fields[13].Label)
		is.Equal("activated_at", m.Fields[13].DBName)
		is.Equal("ActivatedAt", m.Fields[13].Name)

		r1 := NewRow(m.Fields, sqla.Samples[0])
		is.Equal("foo", r1.Fields[1].Value)
		// is.True(r1["is_normal"].(bool))

		is.Equal("3", r1.GetPkValue())
		is.Equal(map[string]string{"id": "3"}, m.where("3"))
	}

	// protype
	o := any(sqla.AllTyped{Name: ""})
	rv := reflect.ValueOf(o)
	for _, f := range m.schema.Fields {
		fv := rv.FieldByName(f.Name)
		is.True(fv.IsValid())
		// fmt.Printf("%s %v\n", f.Name, fv.Interface())
	}
	// is.NotNil(v1)

	// dv1 := r1.GetDisplayValue(m.Fields[8])
	// is.Equal("9527", dv1)

	db, _ := gorm.Open(sqlite.Open(":memory:"),
		&gorm.Config{NamingStrategy: Namer})
	db.AutoMigrate(sqla.AllTyped{})

	a2 := []*sqla.AllTyped{sqla.Samples[0].(*sqla.AllTyped), sqla.Samples[1].(*sqla.AllTyped)}
	tx0 := db.Model(&sqla.AllTyped{}).Create(&a2)
	is.Nil(tx0.Error)

	m1 := NewRow(m.Fields, a2[0])
	rowid := m1.GetPkValue()

	// update
	m1.Map["email"] = "reachable@foo.com"
	tx2 := db.Model(o).Where(m.where(rowid)).Updates(&m1.Map)
	is.Nil(tx2.Error)

	// getOne
	out1 := m.new()
	tx1 := db.Where(m.where(rowid)).First(out1)
	is.Nil(tx1.Error)
	oa1, ok := out1.(*sqla.AllTyped)
	is.True(ok)
	is.Equal(m1.Map["email"], *oa1.Email)
	is.Equal("foo", oa1.Name)

	row := NewRow(m.Fields, out1)
	fid := row.Fields[0].Value
	is.NotZero(fid)

	// create
	m1.Map = map[string]any{"long": "long text", "type": "editor", "email": "a@b.com",
		"age": 3, "is_normal": false, "valid": true, "badge": "doctor", "birthday": "1920-12-01",
		"activated_at": "2000-12-02", "decimal": 12.3, "bytes": []byte("hexed"), "favorite": "book",
		"not_none":   1,
		"last_login": "2000-12-03", "name": "duo"}
	tx3 := db.Model(out1).Clauses(clause.Returning{}).Create(&m1.Map)
	is.Nil(tx3.Error)
	is.NotZero(m1.Map["id"]) // TODO: not return id?

	ar := Wrap(indirect(out1)) // TODO
	is.Equal("alltyped", ar.Endpoint())
	is.Equal("3", ar.GetPkValue())

	// acct := sqla.Account{Addresses: []sqla.Address{{Number: "12321"}, {Number: "22321"}}}
}

func TestWidget(t *testing.T) {
	// is := assert.New(t)
	m := NewModel(sqla.AllTyped{})
	NewForm(m.Fields, nil, "xx")
}

type ModelTestSuite struct {
	suite.Suite
	is        *assert.Assertions
	admin     *Admin
	typedView *ModelView
}

func (ts *ModelTestSuite) SetupTest() {
	ts.is = assert.New(ts.T())

	db, _ := gorm.Open(sqlite.Open(":memory:"),
		&gorm.Config{
			NamingStrategy: Namer,
			Logger:         logger.Default.LogMode(logger.Info),
		})
	ts.admin = NewAdmin("Test Site")
	ts.admin.trace = false

	var c int64
	tx := db.Model(&sqla.Company{}).Count(&c)
	if tx.Error != nil || c == 0 {
		db.AutoMigrate(sqla.Models...)

		samples := []any{
			&sqla.Company{Name: "talk ltd"},
			&sqla.Company{Name: "chat ltd"},
			&sqla.Employee{Name: "Alice", CompanyId: null.NewInt(1, true)},
			&sqla.Employee{Name: "Bob", CompanyId: null.NewInt(1, true)},
		}
		for _, o := range samples {
			tx := db.Create(o)
			if tx.Error != nil {
				panic(tx.Error)
			}
		}
	}

	for _, v := range views(db) {
		ts.admin.AddView(v)
	}

	ts.typedView = ts.admin.FindView("alltyped").(*ModelView)
}

func TestModelTestSuite(t *testing.T) {
	suite.Run(t, new(ModelTestSuite))
}

func (ts *ModelTestSuite) TestRelations() {
	av := ts.admin.FindView("account").(*ModelView)
	fa := av.Model.Fields[2]
	ts.is.Equal("", fa.DBName)
	ts.is.Equal("account", fa.Schema.Table)
	ts.is.Equal("Addresses", fa.Label)
	ts.is.Equal("Addresses", fa.StructField.Name)
}

func (ts *ModelTestSuite) TestAdmin() {
	ts.is.NotNil(ts.admin.FindView("alltyped"))
}

func (ts *ModelTestSuite) TestModel() {

}

func (ts *ModelTestSuite) TestModelView() {
	v := ts.typedView

	ts.is.NotEmpty(v.GetBlueprint().Children)

	ts.is.Equal("/admin/alltyped/", must(v.Blueprint.GetUrl(".index_view")))
	ts.is.Equal("/admin/alltyped/action", must(v.Blueprint.GetUrl(".action")))
	ts.is.Equal("/admin/alltyped/action?a=b", must(v.Blueprint.GetUrl(".action", "a", "b")))

	// query
	r1 := httptest.NewRequest("", "/admin/tag/?sort=0&desc=1&page_size=23&page=2", nil)
	q1 := v.queryFrom(r1)
	ts.is.Equal("0", q1.Sort)
	ts.is.Equal(true, q1.Desc)
	ts.is.Equal(23, q1.PageSize)
	ts.is.Equal(2, q1.Page)

	r2 := httptest.NewRequest("", "/admin/tag/?sort=1", nil)
	q2 := v.queryFrom(r2)
	ts.is.Equal("1", q2.Sort)
	ts.is.Equal(false, q2.Desc)
	ts.is.Equal(20, q2.PageSize)
	ts.is.Equal(0, q2.Page)

	r3 := httptest.NewRequest("", "/admin/tag/details?id=6&url=%2Fadmin%2Ftag%2F%3Fdesc%3D1%26sort%3D1", nil)
	q3 := v.queryFrom(r3)
	ts.is.Equal("", q3.Sort)
	ts.is.Equal(false, q3.Desc)
	ts.is.Equal(20, q3.PageSize)
	ts.is.Equal(0, q3.Page)
	ts.is.Equal("6", q3.Get("id"))
	ts.is.Equal("/admin/tag/?desc=1&sort=1", q3.Get("url"))
}

// func (S *ModelTestSuite) TestSession() {
// 	is := assert.New(S.T())
// 	S.admin.Register(&Blueprint{Endpoint: "bar", Path: "/bar",
// 		Handler: func(w http.ResponseWriter, r *http.Request) {
// 			CurrentSession(r).Set("bar", "hello")
// 		}})
// 	r := httptest.NewRequest("GET", "/admin/bar", nil)
// 	w := httptest.NewRecorder()
// 	S.admin.ServeHTTP(w, r)
// 	is.Equal(200, w.Code)
// }

func (ts *ModelTestSuite) TestUrlStatusCode() {
	ts.is.Equal("/admin/alltyped/", must(ts.admin.UrlFor("", "alltyped.index")))

	es := []string{
		"alltyped",
		"company", "employee",
		"creditcard", "user", "address", "account",
		"language", "student", "toy", "dog",
	}

	type cp struct {
		code int
		path string
	}
	cases := lo.FlatMap(es, func(e string, _ int) []cp {
		return []cp{
			{200, fmt.Sprintf("/admin/%s/", e)},
			{200, fmt.Sprintf("/admin/%s/new", e)},
			{302, fmt.Sprintf("/admin/%s/edit", e)},
			{302, fmt.Sprintf("/admin/%s/details", e)},
			// {302, fmt.Sprintf("/admin/%s/edit?id=1", e)},
			// {200, fmt.Sprintf("/admin/%s/details?id=1", e)},
			{302, fmt.Sprintf("/admin/%s/action", e)},
			{302, fmt.Sprintf("/admin/%s/delete?id=0", e)},
			{200, fmt.Sprintf("/admin/%s/export", e)},
		}
	})
	cases = append(cases, []cp{
		{200, "/admin/console"},
		{200, "/admin/generate"},
		{200, "/admin/trace"},
	}...)

	loop := func() {
		for _, cu := range cases {
			r := httptest.NewRequest("GET", cu.path, nil)
			w := httptest.NewRecorder()
			ts.admin.ServeHTTP(w, r)
			ts.is.Equal(cu.code, w.Code, cu.path)
		}
	}
	loop()

	vc, _ := ts.admin.FindView("company").(*ModelView)
	vc.Joins("Company")

	va, _ := ts.admin.FindView("account").(*ModelView)
	va.Preloads("Addresses")

	loop()

	// for _, p := range sqla.Samples {
	// 	tx := vc.db.Create(p)
	// 	ts.is.Nil(tx.Error)
	// }
	// loop()
}
