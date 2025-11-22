package gadm

import (
	"net/http"
	"time"

	"gopkg.in/guregu/null.v4"
)

type Security struct {
	*BaseView
	CurrentUser *User
}

func AddSecurity(admin *Admin) *Security {
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

	admin.Register(S.Blueprint)

	tm := &Menu{Name: "Theme", Category: "Theme"}
	for _, name := range themes {
		tm.Children = append(tm.Children, &Menu{
			Name: name,
			Path: must(S.Blueprint.GetUrl("admin.theme", "name", name))})
	}

	S.Menu.AddMenu(tm, "Account")
	return S
}

func (S *Security) loginHandler(w http.ResponseWriter, r *http.Request)            {}
func (S *Security) logoutHandler(w http.ResponseWriter, r *http.Request)           {}
func (S *Security) registerHandler(w http.ResponseWriter, r *http.Request)         {}
func (S *Security) forgotPasswordHandler(w http.ResponseWriter, r *http.Request)   {}
func (S *Security) sendConfirmationHandler(w http.ResponseWriter, r *http.Request) {}

type User struct {
	Id    int    `gorm:"primaryKey;autoincrement"`
	Email string `gorm:"uniqueIndex;not null;size:255"`
	// Username is important since shouldn't expose email to other users in most cases.
	Username string `gorm:"size:255"`
	Password string `gorm:"not null;size:255"`
	Active   bool   `gorm:"not null"`

	CreateDatetime time.Time
	UpdateDatetime time.Time

	// Flask-Security user identifier
	// Uniquifier string `gorm:"uniqueIndex;not null"`

	// confirmable
	ConfirmedAt null.Time

	// trackable
	LastLoginAt    null.Time
	CurrentLoginAt null.Time
	LastLoginIp    null.String `gorm:"size:64"`
	CurrentLoginIp null.String `gorm:"size:64"`
	LoginCount     int
	Roles          []Role `gorm:"many2many:user_role"`
}

type Role struct {
	Id          int    `gorm:"primaryKey;autoincrement"`
	Name        string `gorm:"uniqueIndex;not null;size:64"`
	Description string `gorm:"size:255"`
}
