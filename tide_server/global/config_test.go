package global

import (
	"testing"

	"github.com/jackc/pgx/v5"
)

// 连接串里的每个值都必须原样到达驱动：空密码、带空格/引号/反斜杠的密码都不能
// 让后面的参数错位。用 pgx 自己的解析器校验。
func TestPgConfigDSNRoundTrip(t *testing.T) {
	passwords := []string{"", "plain", "with space", `quo'te`, `back\slash`, `mix ' \ = end`}
	for _, password := range passwords {
		cfg := pgConfigStruct{Host: "db.example", Port: "5433", User: "tide user", Password: password, DBName: "tide_gauge"}
		parsed, err := pgx.ParseConfig(cfg.DSN(""))
		if err != nil {
			t.Fatalf("password %q: ParseConfig error: %v", password, err)
		}
		if parsed.Host != "db.example" || parsed.Port != 5433 || parsed.User != "tide user" ||
			parsed.Password != password || parsed.Database != "tide_gauge" {
			t.Fatalf("password %q: parsed host=%q port=%d user=%q password=%q database=%q",
				password, parsed.Host, parsed.Port, parsed.User, parsed.Password, parsed.Database)
		}
	}

	parsed, err := pgx.ParseConfig(pgConfigStruct{Host: "h", Port: "5432", User: "u", DBName: "configured"}.DSN("override"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Database != "override" {
		t.Fatalf("database = %q, want the explicit override", parsed.Database)
	}
}
