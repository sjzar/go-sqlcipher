package main

import (
	"database/sql"

	_ "github.com/sjzar/go-sqlcipher"
)

func main() {
	for _, driver := range sql.Drivers() {
		println(driver)
	}
}
