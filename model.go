package gadm

import (
	"database/sql/driver"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fatih/camelcase"
	"github.com/samber/lo"
	"github.com/spf13/cast"
	"gorm.io/gorm/schema"
)

func fieldValue(a any, name string) any {
	rv := reflect.ValueOf(a)
	for rv.Kind() == reflect.Pointer {
		v := rv.Elem()
		if !v.IsValid() {
			return nil
		}
		rv = reflect.ValueOf(v.Interface())
	}

	if rv.Kind() == reflect.Struct {
		if v := rv.FieldByName(name); v.IsValid() {
			return v.Interface()
		}
	} else if rv.Kind() == reflect.Slice {
		return rv.Interface()
	}
	return nil
}

func (r *Row) Set(field *Field, v any) {
	// How to set struct field?
	r.Map[field.DBName] = v
}

// in detail/edit url is: id=pk1,pk2
func (r *Row) GetPkValue() string {
	vs := []string{}
	for _, f := range r.Fields {
		if f.PrimaryKey {
			vs = append(vs, cast.ToString(f.Value))
		}
	}
	return strings.Join(vs, ",")
}

func (r *Row) FieldOf(col *Field) *Field {
	for _, f := range r.Fields {
		if f.Field == col.Field {
			return f
		}
	}
	return nil
}

type Model struct {
	schema *schema.Schema
	Fields []*Field
}

// used in Open, Struct to table name
var Namer = schema.NamingStrategy{SingularTable: true}
var schemaStore = sync.Map{}

func NewModel(m any) *Model {
	s := must(schema.Parse(m, &schemaStore, Namer))
	fs := lo.Map(s.Fields, func(field *schema.Field, _ int) *Field {
		return &Field{Field: field, Label: strings.Join(camelcase.Split(field.Name), " ")}
	})
	return &Model{schema: s, Fields: fs}
}

// like:
// user: schema.NamingStrategy{SingularTable: true}
// users: schema.NamingStrategy{SingularTable: false}
// used in Blueprint's {Path/Endpoint}
func (m *Model) name() string { return m.schema.Table }

func (m *Model) endpoint() string { return strings.ReplaceAll(m.schema.Table, "_", "") }

func (m *Model) label() string { return strings.Join(camelcase.Split(m.schema.Name), " ") }

// new t
func (m *Model) new() any {
	return reflect.New(m.schema.ModelType).Interface()
}

// new []t
func (m *Model) newSlice() reflect.Value {
	return reflect.New(reflect.SliceOf(m.schema.ModelType))
}

func (m *Model) find(name string) *Field {
	if f, ok := lo.Find(m.Fields, func(field *Field) bool {
		return field.DBName == name
	}); ok {
		return f
	}
	return nil
}

// Return all field can be sorted
// exclude relationship fields
func (m *Model) sortableColumns() []string { return m.schema.DBNames }

// single primarykey, rowid: id
// multiple primarykey, rowid like: pk1,pk2
// return where condition map
func (m *Model) where(rowid string) map[string]string {
	vs := strings.Split(rowid, ",")
	res := map[string]string{}
	if len(m.schema.PrimaryFields) == len(vs) {
		for i := range vs {
			res[m.schema.PrimaryFields[i].DBName] = vs[i]
		}
	}
	return res
}

type Choice struct {
	Value any    `json:"value"`
	Label string `json:"text"` // in flask
}

type Field struct {
	*schema.Field
	Label       string
	Choices     []Choice
	Description string
	TextAreaRow int
	TimeFormat  string
	Readonly    bool // for primary key
	Hidden      bool // for csrf token, TODO: remove, only in form
	Value       any
	Sortable    bool
}

func (f *Field) Endpoint() string {
	if f.Schema == nil {
		panic("not refer field")
	}
	tn := Namer.TableName(f.StructField.Name) // f.Schema.Table) // f.StructField.Name)
	return strings.ReplaceAll(tn, "_", "")
}

func (f *Field) IsSlice() bool {
	rv := reflect.ValueOf(f.Value)
	return rv.Kind() == reflect.Slice
}
func (f *Field) Slice() []*wrap {
	rv := reflect.ValueOf(f.Value)

	ws := make([]*wrap, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		ws[i] = Wrap(rv.Index(i).Interface())
	}
	return ws
}
func (f *Field) IsStruct() bool {
	return f.DBName == "" && f.Schema != nil && !f.IsSlice()
}

func (f *Field) IsNil() bool {
	return isNil(f.Value)
}

// Struct's PkValue
func (f *Field) GetPkValue() string {
	vs := []string{}
	for _, pkf := range f.Schema.PrimaryFields {
		v := fieldValue(f.Value, pkf.Name)
		vs = append(vs, cast.ToString(v))
	}
	return strings.Join(vs, ",")
}

// Edit or display in HTML
// Empty string means nil = sql null, null.String.Valid = false
func (f *Field) Display() string {
	// relation field
	if f.DBName == "" && f.DataType == "" {
		// strong nil check
		if !isNil(f.Value) {
			if str, ok := f.Value.(fmt.Stringer); ok && str != nil {
				return str.String()
			}
		}
		return f.Label
	}

	// bool type field
	if f.DataType == schema.Bool {
		switch v := f.Value.(type) {
		case nil:
			return "false"
		case bool:
			return f.displayBool(v)
		case *bool:
			return f.displayBool(*v)
		}
	}

	switch v := f.Value.(type) {
	case driver.Valuer:
		if isNil(v) {
			return ""
		}

		dv, _ := v.Value()
		if dv == nil {
			return ""
		}
		return cast.ToString(dv)
	case string:
		return v
	case *string:
		if v != nil {
			return *v
		}
		return ""
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64, []byte:
		return cast.ToString(v)
	case time.Time:
		return f.displayTime(v)
	case *time.Time:
		if v != nil {
			return f.displayTime(*v)
		}
		return ""
	case nil:
		return ""
	default:
		log.Printf("todo: %s %v %t\n", f.Name, v, v)
	}
	return ""
}

func (f *Field) displayBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (f *Field) displayTime(t time.Time) string {
	switch f.TimeFormat {
	default:
		return t.Format(time.DateOnly)
	case "YYYY-MM-DD":
		return t.Format(time.DateOnly)
	case "YYYY-MM-DD HH:mm:ss":
		return t.Format(time.DateTime)
	case "HH:mm:ss":
		return t.Format(time.TimeOnly)
	}
}

type wrap struct {
	Model *Model
	Row   *Row
}

func Wrap(v any) *wrap {
	m := NewModel(v)
	return &wrap{m, NewRow(m.Fields, v)}
}

// TODO: add wrap.Display for Stringer
func (w *wrap) Endpoint() string {
	return w.Model.endpoint()
}
func (w *wrap) GetPkValue() string {
	return w.Row.GetPkValue()
}

func indirect(a any) any {
	rv := reflect.Indirect(reflect.ValueOf(a))
	return rv.Interface()
}

type Row struct {
	Fields []*Field
	Map    map[string]any
}

func NewRow(fs []*Field, a any) *Row {
	// need clone, change Value
	fs = clone(fs)
	lo.ForEach(fs, func(f *Field, _ int) {
		f.Value = fieldValue(a, f.Name)
	})
	return &Row{fs, map[string]any{}}
}
func NewSubRow(a any, fields []*Field) *Row {
	fs := fields[:]
	for _, f := range fs {
		f.Value = fieldValue(a, f.Name)
	}
	return &Row{fs, map[string]any{}}
}

func clone(fs []*Field) []*Field {
	cs := make([]*Field, len(fs))
	for i, f := range fs {
		cs[i] = new(Field)
		*cs[i] = *f
	}
	return cs
}

func like(query string) string {
	return "%" + query + "%"
}
