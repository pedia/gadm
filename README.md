<span style="font-size:48pt;color:red">gadm</span>

gadm generates a friendly admin UI for your database with minimal code. It is inspired by Flask-Admin but written for Go and integrates with GORM.

Key goals:
- Easy to edit, filter and search for CRUD and basic relations
- Write less code
- Put everything in one admin
- Auto-generate models(by most powerful `sqlparser-rs`) and views from a live database
- TODO: WebAuthn level safety

![Screenshot](screenshot.png)

Quick example
```go
mv := gadm.NewModelView(User{}).
    SetColumnList("type", "first_name", "last_name", "email", "timezone", "phone_number").
    SetColumnEditableList("first_name", "type", "timezone").
    SetColumnDescriptions(map[string]string{"type": "editor will cause ..."}).
    SetCanSetPageSize(true).
    SetPageSize(5).
    SetTablePrefixHtml(`<h4>Caution ...</h4>`)
admin.AddView(mv)
```

Table generation (UI)
1. Run the generator web UI:

```bash
go run ./cmd
# open http://127.0.0.1:3333/admin/generate
```

2. Enter a database URL such as:

- `sqlite:/absolute/path/to/db.sqlite`
- `postgresql://user:password@localhost:5432/dbname`
- `mysql://user:password@localhost:3306/dbname?charset=utf8mb4&parseTime=True&loc=Local`

3. Click "Generate" — gadm will inspect the schema and produce model/view code you can copy into your project.

How it works (brief)
- gadm maps database tables to ModelViews. Each view exposes routes and templates for list, edit, details, delete, and export.
- The generator inspects the DB via GORM's migrator and builds a simple Go struct representation plus GORM tags.
- Frontend templates are adapted from Flask-Admin (Bootstrap 4) and are included in `static/` and `templates/`.

Features
- Auto-generate Go structs and GORM tags from an existing database
- Configurable column lists, editable fields and descriptions
- Relation support (foreign keys shown as related views)
- Server-side pagination, search, sorting and basic filters
- Extensible actions and custom views
- SQL console and trace GORM SQL Trace per url

When to auto-generate vs write code manually
- Auto-generate when you want a quick admin surface and iterative exploration.
- Prefer handwritten models and views for long-lived, production-critical code — generated code becomes your responsibility to maintain.

Contributing
- File issues or PRs against this repo.
- Keep changes small and focused. Add tests for new features where possible.

Notes and TODO
- Improve type mapping from SQL -> Go types
- Better escaping for default values and comments in generated tags
- Add more examples in `examples/`
- Security and account control

License
This project is licensed under the terms in `LICENSE`.
