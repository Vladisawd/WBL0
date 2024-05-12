package main

import (
	"database/sql"
	"fmt"
)

func dbConnect(config setting) *sql.DB {
	db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable", config.Postgres.PgHost, config.Postgres.PgPort, config.Postgres.PgBase, config.Postgres.PgUser, config.Postgres.PgPassword))
	if err != nil {
		panic(fmt.Sprintf("No connection: %s", err.Error()))
	}

	if err = db.Ping(); err != nil {
		panic(fmt.Sprintf("No connection: %s", err.Error()))
	}

	return db
}
