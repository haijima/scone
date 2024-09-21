package main

import (
	"fmt"
	"loop/other"

	"github.com/jmoiron/sqlx"
)

type User struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

var db *sqlx.DB

func main() {
	db = sqlx.MustOpen("mysql", "user:password@/dbname")
	for _, i := range []int{1, 2, 3} {
		q := "SELECT * FROM users WHERE id = ?"
		var user User
		_ = db.Select(&user, q, i)
		fmt.Println(user)

		query()
		noQuery()

		other.Query()
		other.NoQuery()
	}

	query()
}

func query() {
	_ = db.Select(db, "SELECT * FROM users")
}

func noQuery() {
	fmt.Println("No query")
}
