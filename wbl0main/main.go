package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"text/template"

	"github.com/go-playground/validator/v10"
	_ "github.com/lib/pq"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/stan.go"
)

type order struct {
	OrderUID    string `json:"order_uid" validate:"required"`
	TrackNumber string `json:"track_number" validate:"required"`
	Entry       string `json:"entry" validate:"required"`
	Delivery    struct {
		Name    string `json:"name" validate:"required"`
		Phone   string `json:"phone" validate:"required"`
		Zip     string `json:"zip" validate:"required"`
		City    string `json:"city" validate:"required"`
		Address string `json:"address" validate:"required"`
		Region  string `json:"region" validate:"required"`
		Email   string `json:"email" validate:"required"`
	} `json:"delivery" validate:"required"`
	Payment struct {
		Transaction  string `json:"transaction" validate:"required"`
		RequestID    string `json:"request_id"`
		Currency     string `json:"currency" validate:"required"`
		Provider     string `json:"provider" validate:"required"`
		Amount       int    `json:"amount" validate:"required"`
		PaymentDt    int    `json:"payment_dt" validate:"required"`
		Bank         string `json:"bank" validate:"required"`
		DeliveryCost int    `json:"delivery_cost" validate:"required"`
		GoodsTotal   int    `json:"goods_total" validate:"required"`
		CustomFee    int    `json:"custom_fee"`
	} `json:"payment" validate:"required"`
	Items []struct {
		ChrtID      int    `json:"chrt_id" validate:"required"`
		TrackNumber string `json:"track_number" validate:"required"`
		Price       int    `json:"price" validate:"required"`
		Rid         string `json:"rid" validate:"required"`
		Name        string `json:"name" validate:"required"`
		Sale        int    `json:"sale" validate:"required"`
		Size        string `json:"size" validate:"required"`
		TotalPrice  int    `json:"total_price" validate:"required"`
		NmID        int    `json:"nm_id" validate:"required"`
		Brand       string `json:"brand" validate:"required"`
		Status      int    `json:"status" validate:"required"`
	} `json:"items" validate:"required"`
	Locale            string `json:"locale" validate:"required"`
	InternalSignature string `json:"internal_signature"`
	CustomerID        string `json:"customer_id" validate:"required"`
	DeliveryService   string `json:"delivery_service" validate:"required"`
	Shardkey          string `json:"shardkey" validate:"required"`
	SmID              int    `json:"sm_id" validate:"required"`
	DateCreated       string `json:"date_created" validate:"required"`
	OofShard          string `json:"oof_shard" validate:"required"`
}

var cache = make(map[string]order)

func main() {
	config := newConfig()
	connect := dbConnect(config)

	err := restoreCacheFromDb(connect)
	if err != nil {
		log.Fatalf(err.Error())
	}

	closeNatsStreamingProcess, err := natsStreamingConnect(recordingAnOrder(connect), config)
	if err != nil {
		log.Fatalf(err.Error())
	}

	defer closeNatsStreamingProcess()

	err = HandlerServer(config).ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Unexpected server shutdown: %s", err.Error())
	}
}

// Recovery data to cache from a database
func restoreCacheFromDb(connect *sql.DB) error {

	orders, err := connect.Query(`SELECT "order_id", "order_info" FROM orders`)
	if err != nil {
		return errors.New("сannot read data from database: " + err.Error())
	}
	defer func(orders *sql.Rows) {
		_ = orders.Close()
	}(orders)

	var orderCounter int
	for orders.Next() {
		var orderID string
		var orderInfo []byte
		var order order
		err = orders.Scan(&orderID, &orderInfo)
		if err != nil {
			return errors.New("Failed to scan orders: " + err.Error())
		}
		err := json.Unmarshal(orderInfo, &order)
		if err != nil {
			return errors.New("Failed to parse JSON: " + err.Error())
		}
		cache[orderID] = order
		orderCounter++
	}
	log.Printf("Cache recovery. Recovery: %v orders", orderCounter)
	return nil
}

func natsStreamingConnect(recordingAnOrder func(message *stan.Msg), config setting) (func(), error) {

	natsCon, err := nats.Connect(fmt.Sprintf("nats://%s", config.Nats.Server))
	if err != nil {
		return nil, errors.New("Failed to connect to NATS Streaming server: " + err.Error())
	}

	stanCon, err := stan.Connect(config.Nats.StanClusterID, config.Nats.ClientID, stan.NatsConn(natsCon))
	if err != nil {
		return nil, errors.New("Failed to connect to NATS Streaming channel: " + err.Error())
	}

	stanCon.NatsConn().AuthRequired()

	//Durable subscription so as not to lose data due to connection problems, etc.
	subscribeNats, err := stanCon.Subscribe(config.Nats.SubscribeSubject, recordingAnOrder, stan.DurableName(config.Nats.SubscribeDurableName))
	if err != nil {
		return nil, errors.New("Failed to subscribe to NATS Streaming channel: " + err.Error())
	}

	closeFunc := func() {
		natsCon.Close()
		stanCon.Close()
		subscribeNats.Unsubscribe()
	}

	return closeFunc, nil
}

func recordingAnOrder(connect *sql.DB) func(message *stan.Msg) {
	return func(message *stan.Msg) {

		var newOrder order

		//receiving an order from a channel
		err := json.Unmarshal(message.Data, &newOrder)
		if err != nil {
			log.Printf("Failed to parse JSON: %s", err.Error())
		}

		//order validation
		validate := validator.New()

		err = validate.Struct(newOrder)
		if err != nil {
			log.Printf("Failed to validate JSON: %s", err.Error())
			return
		}

		newOrderInfo, err := json.Marshal(newOrder)
		if err != nil {
			panic(fmt.Sprintf("json coding failed: %s", err.Error()))
		}

		_, ok := cache[newOrder.OrderUID]
		if !ok {
			var mutex sync.Mutex

			//adding an address to the cache and database
			mutex.Lock()
			cache[newOrder.OrderUID] = newOrder

			connect.QueryRow(fmt.Sprintf(`INSERT INTO "orders" ("order_id","order_info") VALUES('%s','%s')`, newOrder.OrderUID, newOrderInfo))
			log.Printf("Added order uid: %s", newOrder.OrderUID)

			mutex.Unlock()
		} else {
			log.Printf("The order under uid: %s is already in the database", newOrder.OrderUID)
			return
		}
	}
}

func HandlerServer(config setting) *http.Server {
	mx := http.NewServeMux()

	mx.HandleFunc("/receiving", GetOrder)
	mx.HandleFunc("/health_check", HealthCheckHandler)

	srv := &http.Server{
		Addr:    config.HttpServer,
		Handler: mx,
	}

	return srv
}

// Outputting data to an html form
func GetOrder(w http.ResponseWriter, r *http.Request) {
	template, err := template.ParseFiles("web/main.html")
	if err != nil {
		http.Error(w, "Parse html file error: "+err.Error(), 400)
		return
	}

	uuid := r.FormValue("uuid")

	orderJson := jsonFormattingOrder(uuid)

	template, _ = template.ParseFiles("web/main.html")

	err = template.Execute(w, orderJson)
	if err != nil {
		http.Error(w, "Failed to execute temp: "+err.Error(), 400)
		return
	}
}

// Checking the received key for availability in the cache
func isValidOrderUuid(uuid string) (bool, error) {
	if uuid == "" {
		return false, errors.New("enter uid")
	}
	_, ok := cache[uuid]
	if !ok {
		return false, errors.New("there is no such uid, enter another one")
	}
	return true, nil
}

func jsonFormattingOrder(uuid string) string {
	ok, err := isValidOrderUuid(uuid)
	if !ok {
		return err.Error()
	}

	order := cache[uuid]
	orderJsonNoFormat, err := json.Marshal(order)

	if err != nil {
		panic(fmt.Sprintf("json coding failed: %s", err.Error()))
	}

	var orderJsonFormat bytes.Buffer

	//json formatting
	err = json.Indent(&orderJsonFormat, orderJsonNoFormat, "", " ")

	if err != nil {
		log.Printf("JSON parse error: %s", err.Error())
		return ""
	}

	return string(orderJsonFormat.Bytes())
}

// Checking the server operation
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(200)
	_, _ = w.Write([]byte("."))
}
