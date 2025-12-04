package gadm

import "testing"

func TestDatabaseURL(t *testing.T) {
	testcases := []struct {
		url string
		dsn string
	}{
		{url: "sqlite:path/to/sqlite.db", dsn: "path/to/sqlite.db"},
		{url: "sqlite::memory:", dsn: ":memory:"},
		{url: "postgres://localhost:5432/mydatabase?user=myuser&password=mypassword",
			dsn: "host=localhost user=myuser password=mypassword dbname=mydatabase port=5432 sslmode=disable"},
		{url: "postgres://user:password@localhost:5432/database",
			dsn: "host=localhost user=user password=password dbname=database port=5432 sslmode=disable"},
		{url: "mysql://user:password@localhost:3366/database?charset=utf8mb4&parseTime=True&loc=Local",
			dsn: "user:password@localhost:3366/database?charset=utf8mb4&parseTime=True&loc=Local"},
		{url: "mysql://localhost:3366/mydatabase?user=myuser&password=mypassword",
			dsn: "myuser:mypassword@localhost:3366/mydatabase?user=myuser&password=mypassword"},
	}
	for _, testcase := range testcases {
		du := Parse(testcase.url)
		if du.DSN != testcase.dsn {
			t.Errorf("expect: %s actual: %s", du.DSN, testcase.dsn)
		}
	}
}
