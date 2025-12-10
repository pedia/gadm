package gadm

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"text/template"
	"time"

	"github.com/go-playground/form/v4"
	"github.com/samber/lo"
	"gopkg.in/guregu/null.v4"
	"gorm.io/gorm"
)

type Security struct {
	*BaseView
	db *gorm.DB
}

func NewSecurity(admin *Admin, db *gorm.DB) *Security {
	cate := "Account"

	S := &Security{db: db}
	S.BaseView = NewView(Menu{Name: gettext("Account")})
	S.Blueprint = &Blueprint{
		Endpoint: "security",
		Path:     "/security",
		Children: map[string]*Blueprint{
			"login":             {Endpoint: "login", Path: "/login", Handler: S.loginHandler},
			"logout":            {Endpoint: "logout", Path: "/logout", Handler: S.logoutHandler},
			"register":          {Endpoint: "register", Path: "/register", Handler: S.registerHandler},
			"forgot_password":   {Endpoint: "forgot_password", Path: "/forgot_password", Handler: S.forgotPasswordHandler},
			"send_confirmation": {Endpoint: "send_confirmation", Path: "/send_confirmation", Handler: S.sendConfirmationHandler},
		},
	}

	if db != nil {
		db.AutoMigrate(&User{}, &Role{})
		var c int64
		if db.Model(Role{}).Count(&c).Error == nil && c == 0 {
			// create demo data
			ra := Role{ID: 1, Name: "admin"}
			rn := Role{ID: 2, Name: "user"}
			db.Create(&ra)
			db.Create(&rn)
			db.Create(&User{Email: "admin@gadm.com",
				Password: password_hash("admin"),
				Active:   true,
				Roles:    []Role{ra, rn}})
		}
	}

	admin.AddView(S)

	tm := &Menu{Name: "Theme"}
	for _, name := range themes {
		tm.Children = append(tm.Children, &Menu{
			Name: name,
			Path: must(S.Blueprint.GetUrl("admin.theme", "name", name))})
	}
	S.Menu.AddMenu(tm, cate)

	admin.AddView(NewModelView(Role{}, db), cate)
	vu := NewModelView(User{}, db).
		Preloads("Roles").
		SetColumnEditableList("email", "active").
		SetColumnSearchableList("email", "username")
	admin.AddView(vu, cate)

	S.Menu.Children = append(S.Menu.Children,
		&Menu{Name: "SQL Console", Path: must(S.Blueprint.GetUrl("admin.console"))},
		&Menu{Name: "Trace", Path: must(S.Blueprint.GetUrl("admin.trace"))},
		&Menu{Name: "Generate", Path: must(S.Blueprint.GetUrl("admin.generate"))})
	return S
}

func (S *Security) logoutHandler(w http.ResponseWriter, r *http.Request) {
	S.logout(r)
	http.Redirect(w, r, must(S.Blueprint.GetUrl("admin.index")), http.StatusFound)
}
func (S *Security) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()

		email := r.PostFormValue("email")
		password := r.PostFormValue("password")
		if email == "" || password == "" {
			S.AddFlash(r, FlashInfo(gettext("Miss email or password")))
		} else {
			var u User
			tx := S.db.Preload("Roles").Find(&u, "email=? and password=? and active=true",
				email, password_hash(password))
			if tx.Error != nil || u.ID == 0 {
				S.AddFlash(r, FlashInfo("Email or password wrong"))
			} else {
				S.AddFlash(r, FlashInfo(gettext("Logined")))
				S.makeLogin(r, u.ID)
				http.Redirect(w, r, must(S.Blueprint.GetUrl("admin.index")), http.StatusFound)
			}
		}
	}

	S.Render(w, r, "templates/security/login_user.tmpl",
		template.FuncMap{
			// "csrf_token":           func() string { return csrf.Token(r) },
			// "get_flashed_messages": func() []any { return S.admin.Session(r).Flashes() },
			// "gettext": gettext,
			// "get_url": S.Blueprint.GetUrl,
		}, map[string]any{
			"path":      r.URL.Path,
			"name":      S.Menu.Name,
			"extra_css": []string{},
			"extra_js":  []string{}, // "a.js", "b.js"}
			"admin":     S.admin.dict(r),

			"admin_fluid_layout": true,

			"editable_columns": nil,
			// "category":  S.Menu.Category,
			// "name":      S.Menu.Name,
			// "extra_css": []string{},
			// "extra_js":  []string{}, // "a.js", "b.js"}
		})
}
func (S *Security) registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()

		var rf register_form
		err := form.NewDecoder().Decode(&rf, r.PostForm)
		if err != nil {
			FlashError(err)
		} else {
			if rf.Password != rf.ConfirmPassword {
				S.AddFlash(r, FlashInfo(gettext("Confirmed password not match")))
			} else {
				u := User{
					Email:    rf.Email,
					Username: rf.Username,
					Password: password_hash(rf.Password),
				}
				tx := S.db.Create(&u)
				if tx.Error != nil {
					FlashError(tx.Error)
				} else {
					S.AddFlash(r, FlashInfo(gettext("Register success")))
					S.makeLogin(r, u.ID)
					http.Redirect(w, r, must(S.Blueprint.GetUrl("admin.index")), http.StatusFound)
				}
			}
		}
	}

	S.Render(w, r, "templates/security/register_user.tmpl",
		template.FuncMap{
			// "csrf_token":           func() string { return csrf.Token(r) },
			// "get_flashed_messages": func() []any { return S.admin.Session(r).Flashes() },
			// "gettext": gettext,
			// "get_url": S.Blueprint.GetUrl,
		}, map[string]any{
			"path":      r.URL.Path,
			"name":      S.Menu.Name,
			"extra_css": []string{},
			"extra_js":  []string{}, // "a.js", "b.js"}
			"admin":     S.admin.dict(r),

			"admin_fluid_layout": true,

			"editable_columns": nil,
			// "category":  S.Menu.Category,
			// "name":      S.Menu.Name,
			// "extra_css": []string{},
			// "extra_js":  []string{}, // "a.js", "b.js"}
		})
}
func (S *Security) forgotPasswordHandler(w http.ResponseWriter, r *http.Request)   {}
func (S *Security) sendConfirmationHandler(w http.ResponseWriter, r *http.Request) {}

func (S *Security) Check(w http.ResponseWriter, r *http.Request) {
	// if !logined(r) {
	//   redirect to security.login
	// }
}

func (S *Security) getUser(uid int) *User {
	var u User
	if S.db.Preload("Roles").Find(&u, uid).Error == nil {
		return &u
	}
	return nil
}
func (S *Security) makeLogin(r *http.Request, uid int) {
	S.admin.Session(r).Values["uid"] = uid
}
func (S *Security) logout(r *http.Request) {
	delete(S.admin.Session(r).Values, "uid")
}

// return md5(password + "gadm")
func password_hash(t string) string {
	x := md5.New().Sum([]byte(t + ":gadm"))
	return "md5:" + hex.EncodeToString(x)
}

type User struct {
	ID    int    `gorm:"primaryKey;autoincrement"`
	Email string `gorm:"uniqueIndex;not null;size:255"`
	// Username is important since shouldn't expose email to other users in most cases.
	Username    string `gorm:"size:255"`
	Password    string `gorm:"not null;size:255"`
	Active      bool   `gorm:"not null;default:false"`
	ActivatedAt *time.Time

	CreatedAt   *time.Time `gorm:"autoCreateTime"`
	UpdatedAt   *time.Time `gorm:"autoUpdateTime:nano"`
	ConfirmedAt *time.Time

	// trackable
	LastLoginAt    *time.Time
	CurrentLoginAt *time.Time
	LastLoginIp    null.String `gorm:"size:64"`
	CurrentLoginIp null.String `gorm:"size:64"`
	LoginCount     int         `gorm:"default:0"`
	Roles          []Role      `gorm:"many2many:user_role"`
}

type Role struct {
	ID          int    `gorm:"primaryKey;autoincrement"`
	Name        string `gorm:"uniqueIndex;not null;size:64"`
	Description string `gorm:"size:255"`
}

func (r *Role) String() string { return r.Name }

type register_form struct {
	Username        string `form:"username"`
	Email           string `form:"email"`
	Password        string `form:"password"`
	ConfirmPassword string `form:"confirm_password"`
}

type contextKey string

const currentUserKey contextKey = "cu"

func CurrentUser(r *http.Request) *User {
	a := r.Context().Value(currentUserKey)
	if a != nil {
		return a.(*User)
	}
	return nil
}

func CurrentRoles(r *http.Request) []string {
	if u := CurrentUser(r); u != nil {
		return lo.Map(u.Roles, func(r Role, _ int) string {
			return r.Name
		})
	}
	return nil
}
