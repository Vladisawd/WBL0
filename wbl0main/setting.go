package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type setting struct {
	Server     string
	PgHost     string
	PgPort     string
	PgUser     string
	PgPassword string
	PgBase     string
}

func newConf() setting {

	file, err := os.Open("setting.cfg")
	if err != nil {
		panic(fmt.Sprintf("Failed to open file %s", err.Error()))
	}

	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		panic(fmt.Sprintf("Failed to read file information %s", err.Error()))
	}

	fileByte := make([]byte, stat.Size())

	_, err = file.Read(fileByte)
	if err != nil {
		panic(fmt.Sprintf("Failed to read configuration file %s", err.Error()))
	}

	var conf setting

	err = json.Unmarshal(fileByte, &conf)
	if err != nil {
		panic(fmt.Sprintf("Don't count data %s", err.Error()))
	}

	return conf
}
