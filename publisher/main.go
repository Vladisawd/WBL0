package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
)

type order struct {
	OrderUID    string `json:"order_uid"`
	TrackNumber string `json:"track_number"`
	Entry       string `json:"entry"`
	Delivery    struct {
		Name    string `json:"name"`
		Phone   string `json:"phone"`
		Zip     string `json:"zip"`
		City    string `json:"city"`
		Address string `json:"address"`
		Region  string `json:"region"`
		Email   string `json:"email"`
	} `json:"delivery"`
	Payment struct {
		Transaction  string `json:"transaction"`
		RequestID    string `json:"request_id"`
		Currency     string `json:"currency"`
		Provider     string `json:"provider"`
		Amount       int    `json:"amount"`
		PaymentDt    int    `json:"payment_dt"`
		Bank         string `json:"bank"`
		DeliveryCost int    `json:"delivery_cost"`
		GoodsTotal   int    `json:"goods_total"`
		CustomFee    int    `json:"custom_fee"`
	} `json:"payment"`
	Items []struct {
		ChrtID      int    `json:"chrt_id"`
		TrackNumber string `json:"track_number"`
		Price       int    `json:"price"`
		Rid         string `json:"rid"`
		Name        string `json:"name"`
		Sale        int    `json:"sale"`
		Size        string `json:"size"`
		TotalPrice  int    `json:"total_price"`
		NmID        int    `json:"nm_id"`
		Brand       string `json:"brand"`
		Status      int    `json:"status"`
	} `json:"items"`
	Locale            string `json:"locale"`
	InternalSignature string `json:"internal_signature"`
	CustomerID        string `json:"customer_id"`
	DeliveryService   string `json:"delivery_service"`
	Shardkey          string `json:"shardkey"`
	SmID              int    `json:"sm_id"`
	DateCreated       string `json:"date_created"`
	OofShard          string `json:"oof_shard"`
}

// script for publishing data on the channel
func main() {
	natsCon, err := nats.Connect("nats://localhost:4223")
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to NATS Streaming server: %s", err.Error()))
	}

	stanCon, err := stan.Connect("test-cluster", "test-user", stan.NatsConn(natsCon))
	if err != nil {
		panic(fmt.Sprintf("Failed to connect to NATS Streaming channel: %s", err.Error()))
	}

	defer stanCon.Close()

	fileOrders, err := os.ReadFile("json_example.json")
	if err != nil {
		panic(fmt.Sprintf("Failed to read file %s", err.Error()))
	}

	var orders []order

	//reading a list of all orders
	err = json.Unmarshal(fileOrders, &orders)
	if err != nil {
		panic(fmt.Sprintf("json decoding failed: %s", err.Error()))
	}

	for _, order := range orders {

		//pull orders into the channel one at a time
		orderByte, err := json.Marshal(order)
		if err != nil {
			panic(fmt.Sprintf("json coding failed: %s", err.Error()))
		}

		log.Printf("Order under uid: %s sent\n", order.OrderUID)
		stanCon.Publish("test-channel", orderByte)
	}
}
