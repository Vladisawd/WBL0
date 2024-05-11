package main

import (
	"database/sql"
	"fmt"
)

func db_connect(conf setting) *sql.DB {
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable", conf.PgHost, conf.PgPort, conf.PgBase, conf.PgUser, conf.PgPassword))
	if err != nil {
		panic(fmt.Sprintf("No connection: %s", err.Error()))
	}

	if err = db.Ping(); err != nil {
		panic(fmt.Sprintf("No connection: %s", err.Error()))
	}

	return db
}
