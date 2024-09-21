package other

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

func Query() {
	db := sqlx.MustOpen("mysql", "user:password@/dbname")
	_ = db.Select(db, "SELECT * FROM users")
}

func NoQuery() {
	fmt.Println("No query")
}
