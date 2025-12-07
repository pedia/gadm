package gadm

import (
	"errors"
	"fmt"
	"gadm/isdebug"
	"io"
	"log"
	"os"
	"os/exec"
	"plugin"
	"reflect"
	"strings"
	"text/template"
	"time"

	"github.com/fatih/camelcase"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// generator model declare, add to Admin, and link as a plugin
func NewGenerator(url string) *generator {
	return &generator{Url: url,
		Package: "dao",
		logger:  log.New(os.Stdout, "", log.Lmicroseconds),
	}
}

type generator struct {
	Url     string
	Tables  []*schema.Schema
	Package string
	logger  *log.Logger
}

func (g *generator) Run(admin *Admin, w io.Writer) error {
	// safe guard for call from websocket
	if g == nil {
		return errors.New("")
	}
	g.logger.SetOutput(w)

	g.logger.Println("generate start")

	// open db
	db, err := Open(g.Url, &gorm.Config{
		NamingStrategy: Namer,
		Logger: logger.New(g.logger, logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: false,
			Colorful:                  false,
		})})
	if err != nil {
		g.logger.Printf("url %s parse failed", g.Url)
		return err
	}

	g.logger.Printf("database %s opened", g.Url)

	// for _, cs := range must(GetTables(db)) {
	// 	g.logger.Println("found table", cs)
	// 	if doc, err := sqlparser.Parse(db.Dialector.Name(), cs); err == nil {
	// 		_ = doc[0]
	// 	}
	// }

	m := db.Migrator()
	for _, table := range must(m.GetTables()) {
		cts := must(m.ColumnTypes(table))
		idx := must(m.GetIndexes(table))
		g.EmitTable(table, cts, idx)
	}

	if sqldb, err := db.DB(); err == nil {
		_ = sqldb.Close()
	}
	g.logger.Printf("database closed")

	_ = os.Mkdir("dao", 0755)

	g.logger.Printf("write dao/models.gen.go")
	f, err := os.OpenFile("dao/models.gen.go", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	f.WriteString("// generate by gadm.generator\n")
	f.WriteString(fmt.Sprintf("package %s\n\n", g.Package))
	f.WriteString("import \"time\"\n\n")
	g.WriteModels(f)
	f.Close()
	g.logger.Printf("close dao/models.gen.go")

	//
	if err := g.execTemplate("dao/models_test.go", testcode); err != nil {
		return err
	}
	if err := g.execTemplate("dao/views.go", viewscode); err != nil {
		return err
	}

	// go fmt ./dao
	// CGO_CFLAGS="-O0 -g" go build -gcflags="all=-N -l" -o libdao.so -buildmode=plugin ./dao
	// execute ./dao
	g.exec("go", "fmt", "./dao")
	g.exec("go", "test", "./dao")
	// go mod init main?
	// go get gadm?

	if isdebug.On {
		g.exec("go", "build", "-buildmode=plugin",
			`-gcflags`, `all=-N -l`, // debug
			"-tags", "debug", // for isdebug.On
			"-o", "libdao.so", "./dao")
	} else {
		g.exec("go", "build", "-buildmode=plugin",
			"-o", "libdao.so", "./dao")
	}

	if admin != nil {
		if err := LoadPlugin(admin, "libdao.so"); err != nil {
			g.logger.Printf("load plugin failed %s", err)
			return err
		}
		g.logger.Printf("load plugin")
	}
	g.logger.Printf("all done, reload page")
	return nil
}

func (g *generator) exec(name string, arg ...string) error {
	g.logger.Printf("execute %s %v", name, arg)
	cmd := exec.Command(name, arg...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		g.logger.Printf("failed %s %s", err, string(output))
		return err
	}

	g.logger.Println(string(output))
	return nil
}

// post_tags => PostTags
func namify(name string) string {
	a1 := strings.Split(name, "_")
	a2 := []string{}
	for _, a := range a1 {
		a2 = append(a2, camelcase.Split(a)...)
	}
	a3 := lo.Map(a2, func(s string, _ int) string {
		return strings.Title(s)
	})
	return strings.Join(a3, "")
}

func dialectTypeToDataType(n string) schema.DataType {
	n = strings.ToUpper(n)
	table := map[string]schema.DataType{
		"":         schema.String, // CREATE TABLE sqlite_sequence(name,seq);
		"BOOLEAN":  schema.Bool,
		"CHAR":     schema.String,
		"DATE":     schema.Time,
		"DATETIME": schema.Time,
		"INTEGER":  schema.Int,
		"NUMERIC":  schema.Int,
		"DOUBLE":   schema.Float,
		"REAL":     schema.Float,
		"TEXT":     schema.String,
		"VARCHAR":  schema.String,
		"BLOB":     schema.Bytes,
		// "NULL": schema.,
		"TINYINT":            schema.Int,
		"SMALLINT":           schema.Int,
		"MEDIUMINT":          schema.Int,
		"INT":                schema.Int,
		"BIGINT":             schema.Uint,
		"DECIMAL":            schema.Float, // ?
		"FLOAT":              schema.Float,
		"BIT":                schema.Int, // ?
		"TIMESTAMP":          schema.Time,
		"TIME":               schema.Time,
		"YEAR":               schema.String, // ?
		"BINARY":             schema.Bytes,
		"VARBINARY":          schema.Bytes,
		"TINYBLOB":           schema.Bytes,
		"MEDIUMBLOB":         schema.Bytes,
		"LONGBLOB":           schema.Bytes,
		"TINYTEXT":           schema.String,
		"MEDIUMTEXT":         schema.String,
		"LONGTEXT":           schema.String,
		"ENUM":               schema.String, // ?
		"SET":                schema.String, // ?
		"GEOMETRY":           schema.String,
		"POINT":              schema.String,
		"LINESTRING":         schema.String,
		"POLYGON":            schema.String,
		"MULTIPOINT":         schema.String,
		"MULTILINESTRING":    schema.String,
		"MULTIPOLYGON":       schema.String,
		"GEOMETRYCOLLECTION": schema.String,
		"JSON":               schema.String,
	}
	for k, v := range table {
		if k == n || strings.HasPrefix(n, k) {
			return v
		}
	}
	return ""
}

func (g *generator) EmitTable(table string, columns []gorm.ColumnType, indexes []gorm.Index) {
	t := schema.Schema{
		Name:  namify(table),
		Table: table,
	}
	g.Tables = append(g.Tables, &t)
	for _, col := range columns {
		f := schema.Field{
			DBName: col.Name(),
			Name:   namify(col.Name()),
		}
		g.logger.Printf("  %s %s\n", col.Name(), col.DatabaseTypeName())

		if dt := dialectTypeToDataType(col.DatabaseTypeName()); dt != "" {
			f.DataType = dt
			f.GORMDataType = dt

			// special
			if dt == schema.Int {
				// SQLite does not have a separate Boolean storage class
				if dv, ok := col.DefaultValue(); ok && dv == "true" || dv == "false" {
					f.GORMDataType = schema.Bool
				}
			} else if dt == schema.Time {
				f.GORMDataType = schema.DataType("*time.Time")
			} else if dt == schema.Bytes {
				f.GORMDataType = schema.DataType("[]byte")
			}
		}

		if pk, ok := col.PrimaryKey(); ok {
			f.PrimaryKey = pk
		}
		if a, ok := col.AutoIncrement(); ok {
			f.AutoIncrement = a
		}
		if dv, ok := col.DefaultValue(); ok {
			f.DefaultValue = dv
			f.HasDefaultValue = true
		}
		if length, ok := col.Length(); ok {
			f.Size = int(length)
		}
		if nullable, ok := col.Nullable(); ok {
			f.NotNull = !nullable
		}
		if comment, ok := col.Comment(); ok {
			f.Comment = comment
		}

		t.Fields = append(t.Fields, &f)
	}

	for _, idx := range indexes {
		if pk, ok := idx.PrimaryKey(); ok && pk {
			for _, col := range idx.Columns() {
				if f, ok := lo.Find(t.Fields, func(f *schema.Field) bool {
					return f.DBName == col
				}); ok {
					f.PrimaryKey = true
					if unique, ok := idx.Unique(); ok {
						f.Unique = unique
					}
				}
			}
		}
	}

	for _, f := range t.Fields {
		g.applyTag(f)
	}
}

// Build a gorm tag with useful attributes
func (g *generator) applyTag(f *schema.Field) {
	parts := []string{}
	// column name
	// parts = append(parts, fmt.Sprintf("column:%s", f.DBName))

	// type (from dialect mapping)
	// if f.GORMDataType != "" {
	// 	parts = append(parts, fmt.Sprintf("type:%s", f.GORMDataType))
	// }

	// size
	if f.Size > 0 {
		parts = append(parts, fmt.Sprintf("size:%d", f.Size))
	}
	// primary key / auto increment
	if f.PrimaryKey {
		parts = append(parts, "primaryKey")
	}
	if f.AutoIncrement { // TODO:
		parts = append(parts, "autoIncrement")
	}
	// default value (keep as-is)
	if f.HasDefaultValue {
		// default value could contain spaces/quotes; keep it raw
		parts = append(parts, fmt.Sprintf("default:%s", f.DefaultValue))
	}
	// not null
	if f.NotNull {
		parts = append(parts, "not null")
	}

	// unique
	if f.Unique {
		parts = append(parts, "unique")
	}
	// comment (avoid double quotes inside tag)
	if f.Comment != "" {
		safeComment := strings.ReplaceAll(f.Comment, "\"", "'")
		parts = append(parts, fmt.Sprintf("comment:%s", safeComment))
	}

	if len(parts) == 0 {
		return
	}

	tag := strings.Join(parts, ";")
	// f.Tag will be embedded inside backticks when writing the struct
	f.Tag = reflect.StructTag(fmt.Sprintf("gorm:\"%s\"", tag))
}

func (g *generator) WriteModels(w io.Writer) {
	g.logger.Println("write models")
	for _, table := range g.Tables {
		g.writeSchema(w, table)
	}
}

func (g *generator) writeSchema(w io.Writer, table *schema.Schema) {
	fmt.Fprintf(w, "type %s struct {\n", table.Name)
	for _, field := range table.Fields {
		if field.Tag != "" {
			// field.Tag is a reflect.StructTag (underlying string) and may contain quotes; wrap in backticks
			fmt.Fprintf(w, "\t%s %s `%s`\n", field.Name, field.GORMDataType, string(field.Tag))
		} else {
			fmt.Fprintf(w, "\t%s %s\n", field.Name, field.GORMDataType)
		}
	}
	fmt.Fprintf(w, "}\n\n")
}

func (g *generator) execTemplate(fn string, tmpl *template.Template) (err error) {
	f, err := os.OpenFile(fn, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, g); err != nil {
		return err
	}
	return f.Close()
}

var testcode = template.Must(template.New("testcode").Parse(`
package {{.Package}}

import (
	"gadm"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDaoModels(t *testing.T) {
	cwd, _ := os.Getwd()
	if strings.HasSuffix(cwd, "dao") {
		_ = os.Chdir("..")
	}

	db, err := gadm.Parse("{{.Url}}").Open(
		&gorm.Config{
			NamingStrategy: gadm.Namer,
			Logger:         logger.Default.LogMode(logger.Info)})
	if err != nil {
		return
	}

	
	var models = []any{ {{range .Tables}}
		&{{.Name}}{}, 
		{{- end}}}
	for _, m := range models {
		tx := db.First(m)
		if tx.Error != nil && tx.Error != gorm.ErrRecordNotFound {
			t.Error(tx.Error)
		}
	}
}
`))

var viewscode = template.Must(template.New("viewscode").Parse(`
package {{.Package}}

import "gadm"

func Views() []*gadm.ModelView {
	db, err := gadm.Open("{{.Url}})")
	if err != nil {
		return nil
	}
	_ = db

	return []*gadm.ModelView{
	{{range .Tables}}
		gadm.NewModelView({{.Name}}{}, db),
	{{- end}}
	}
}
`))

func LoadPlugin(admin *Admin, fn string) error {
	if _, err := os.Stat(fn); err != nil {
		return err
	}

	p, err := plugin.Open(fn)
	if err != nil {
		return err
	}

	// Add Views
	if f, err := p.Lookup("Views"); err == nil {
		if ft, ok := f.(func() []*ModelView); ok {
			vs := ft()
			for _, v := range vs {
				admin.AddView(v, "Generate")
			}
		}
	}

	return nil
}
