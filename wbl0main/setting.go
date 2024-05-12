package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type setting struct {
	HttpServer string
	Postgres   struct {
		PgHost     string
		PgPort     string
		PgUser     string
		PgPassword string
		PgBase     string
	}
	Nats struct {
		Server               string
		StanClusterID        string
		ClientID             string
		SubscribeSubject     string
		SubscribeDurableName string
	}
}

func newConfig() setting {

	file, err := os.Open("setting.json")
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

	var config setting

	err = json.Unmarshal(fileByte, &config)
	if err != nil {
		panic(fmt.Sprintf("Don't count data %s", err.Error()))
	}

	return config
}
