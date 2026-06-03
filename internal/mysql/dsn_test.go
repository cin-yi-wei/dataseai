package mysql

import "testing"

func TestBuildDSN(t *testing.T) {
	cases := []struct {
		in   DSNInput
		want string
	}{
		{
			DSNInput{Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "disabled"},
			"u:p@tcp(h:3306)/?parseTime=true&tls=false&charset=utf8mb4",
		},
		{
			DSNInput{Host: "h", Port: 3307, Username: "u", Password: "p:@/", DefaultDB: "mydb", TLS: "required"},
			"u:p%3A%40%2F@tcp(h:3307)/mydb?parseTime=true&tls=true&charset=utf8mb4",
		},
		{
			DSNInput{Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "preferred"},
			"u:p@tcp(h:3306)/?parseTime=true&tls=preferred&charset=utf8mb4",
		},
	}
	for _, c := range cases {
		got := BuildDSN(c.in)
		if got != c.want {
			t.Errorf("BuildDSN(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}
