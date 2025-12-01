package gadm

import (
	"net/http"
	"text/template"
	"time"

	"gopkg.in/guregu/null.v4"
	"gorm.io/gorm"
)

type Security struct {
	*BaseView
}

func AddSecurity(admin *Admin, db *gorm.DB) *Security {
	S := new(Security)
	S.BaseView = NewView(Menu{Name: gettext("Account"), Category: "Account"})
	S.Blueprint = &Blueprint{
		Endpoint: "security",
		Children: map[string]*Blueprint{
			"login":             {Endpoint: "login", Path: "/login", Handler: S.loginHandler},
			"logout":            {Endpoint: "logout", Path: "/logout", Handler: S.logoutHandler},
			"register":          {Endpoint: "register", Path: "/register", Handler: S.registerHandler},
			"forgot_password":   {Endpoint: "forgot_password", Path: "/forgot_password", Handler: S.forgotPasswordHandler},
			"send_confirmation": {Endpoint: "send_confirmation", Path: "/send_confirmation", Handler: S.sendConfirmationHandler},
		},
	}

	admin.AddView(S)

	tm := &Menu{Name: "Theme", Category: "Theme"}
	for _, name := range themes {
		tm.Children = append(tm.Children, &Menu{
			Name: name,
			Path: must(S.Blueprint.GetUrl("admin.theme", "name", name))})
	}

	if db != nil {
		db.AutoMigrate(&User{}, &Role{})
	}

	S.Menu.AddMenu(tm, "Account")
	S.Menu.AddMenu(&Menu{Name: "SQL Console", Path: must(S.Blueprint.GetUrl("admin.console"))}, "Account")
	S.Menu.AddMenu(&Menu{Name: "Trace", Path: must(S.Blueprint.GetUrl("admin.trace"))}, "Account")
	S.Menu.AddMenu(&Menu{Name: "Generate", Path: must(S.Blueprint.GetUrl("admin.generate"))}, "Account")
	return S
}

func (S *Security) loginHandler(w http.ResponseWriter, r *http.Request)  {}
func (S *Security) logoutHandler(w http.ResponseWriter, r *http.Request) {}
func (S *Security) registerHandler(w http.ResponseWriter, r *http.Request) {
	S.Render(w, r, "templates/security/register_user.tmpl",
		template.FuncMap{
			// "csrf_token":           func() string { return csrf.Token(r) },
			// "get_flashed_messages": func() []any { return S.admin.Session(r).Flashes() },
			// "gettext": gettext,
			// "get_url": S.Blueprint.GetUrl,
		}, map[string]any{
			"editable_columns": nil,
			// "category":  S.Menu.Category,
			// "name":      S.Menu.Name,
			// "extra_css": []string{},
			// "extra_js":  []string{}, // "a.js", "b.js"}
		})
}
func (S *Security) forgotPasswordHandler(w http.ResponseWriter, r *http.Request)   {}
func (S *Security) sendConfirmationHandler(w http.ResponseWriter, r *http.Request) {}

type User struct {
	Id    int    `gorm:"primaryKey;autoincrement"`
	Email string `gorm:"uniqueIndex;not null;size:255"`
	// Username is important since shouldn't expose email to other users in most cases.
	Username    string `gorm:"size:255"`
	Password    string `gorm:"not null;size:255"`
	Active      bool   `gorm:"not null;default:false"`
	ActivatedAt null.Time

	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime:nano"`

	// confirmable
	ConfirmedAt null.Time

	// trackable
	LastLoginAt    null.Time
	CurrentLoginAt null.Time
	LastLoginIp    null.String `gorm:"size:64"`
	CurrentLoginIp null.String `gorm:"size:64"`
	LoginCount     int         `gorm:"default:0"`
	Roles          []Role      `gorm:"many2many:user_role"`
}

type Role struct {
	Id          int    `gorm:"primaryKey;autoincrement"`
	Name        string `gorm:"uniqueIndex;not null;size:64"`
	Description string `gorm:"size:255"`
}

type register struct {
	Username   string `form:"username"`
	Email      string `form:"email"`
	Password   string `form:"password"`
	RePassword string `form:"repassword"`
}
