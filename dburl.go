package gadm

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// call gorm.Open
func Open(jdbcURL string, opts ...gorm.Option) (*gorm.DB, error) {
	return Parse(jdbcURL).Open(opts...)
}

// JdbcURL Parsed result
type databaseURL struct {
	URL     *url.URL
	DSN     string
	Creator func(string) gorm.Dialector
}

// Open JdbcURL as *gorm.DB
func (du *databaseURL) Open(opts ...gorm.Option) (*gorm.DB, error) {
	nopt := []gorm.Option{&gorm.Config{
		NamingStrategy: Namer,
		Logger:         logger.Default.LogMode(logger.Info)}}
	nopt = append(nopt, opts...)
	return gorm.Open(du.Creator(du.DSN), nopt...)
}

// JdbcURL like:
// sqlite:path/to/sqlite.db
// sqlite::memory:
// postgresql://localhost:5432/mydatabase?user=myuser&password=mypassword
// postgresql://user:password@localhost:5432/database
// mysql://user:password@localhost:3366/database?charset=utf8mb4&parseTime=True&loc=Local
// mysql://localhost:3366/mydatabase?user=myuser&password=mypassword
//
// Parsed it to DatabaseURL, and Open lately
func Parse(jdbc string) *databaseURL {
	parsed, err := url.Parse(jdbc)
	if err != nil {
		return nil
	}

	r := databaseURL{URL: parsed}

	switch parsed.Scheme {
	case "postgresql", "postgres":
		r.Creator = postgres.Open

		// dsn := "host=localhost user=gorm password=gorm dbname=gorm port=9920 sslmode=disable TimeZone=America/New_York"
		arr := []string{}
		host, port, err := net.SplitHostPort(parsed.Host)
		if err != nil {
			host = parsed.Host
			port = "5433" // default
		}
		arr = append(arr, fmt.Sprintf("host=%s", host))

		var user string
		if parsed.User != nil {
			user = parsed.User.Username()
		} else {
			user = parsed.Query().Get("user")
		}
		if user == "" {
			user = "postgresql" // default
		}
		arr = append(arr, fmt.Sprintf("user=%s", user))

		var password string
		if parsed.User != nil {
			password, _ = parsed.User.Password()
		} else {
			password = parsed.Query().Get("password")
		}
		arr = append(arr, fmt.Sprintf("password=%s", password))

		if !strings.HasPrefix(parsed.Path, "/") {
			return nil
		}
		dbname := parsed.Path[1:]
		arr = append(arr, fmt.Sprintf("dbname=%s", dbname))

		arr = append(arr, fmt.Sprintf("port=%s", port))

		sslmode := parsed.Query().Get("sslmode")
		if sslmode == "" {
			sslmode = "disable"
		}
		arr = append(arr, fmt.Sprintf("sslmode=%s", sslmode))

		TimeZone := parsed.Query().Get("TimeZone")
		if TimeZone == "" {
			// TODO: TimeZone = time.Local.String()
		}
		if TimeZone != "" {
			arr = append(arr, fmt.Sprintf("TimeZone=%s", TimeZone))
		}

		r.DSN = strings.Join(arr, " ")
	case "sqlite":
		r.Creator = sqlite.Open
		r.DSN = parsed.Opaque
	case "mysql":
		r.Creator = mysql.Open
		// dsn := "user:password@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local"
		arr := []string{}
		if parsed.User != nil {
			password, _ := parsed.User.Password()
			arr = append(arr, fmt.Sprintf("%s:%s@", parsed.User.Username(), password))
		} else {
			user := parsed.Query().Get("user")
			password := parsed.Query().Get("password")
			if user != "" {
				arr = append(arr, fmt.Sprintf("%s:%s@", user, password))
			}
		}
		arr = append(arr, parsed.Host)
		arr = append(arr, parsed.Path)
		if parsed.RawQuery != "" {
			// TODO: remove user/password
			arr = append(arr, "?"+parsed.RawQuery)
		}
		r.DSN = strings.Join(arr, "")
	}

	return &r
}
