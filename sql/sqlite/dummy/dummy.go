package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	fmt.Println(sql.Open("sqlite3", ":memory:"))
}
