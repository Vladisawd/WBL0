package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
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
	conf := newConf()
	natsCon, stanCon, subscribeNats := natsStreamingConnect(connectDB(conf))
	cacheRecovery(connectDB(conf))
	handler(conf)
	natsCon.Close()
	stanCon.Close()
	subscribeNats.Unsubscribe()
}

func natsStreamingConnect(connectDB *sql.DB) (*nats.Conn, stan.Conn, stan.Subscription) {

	natsCon, err := nats.Connect("nats://localhost:4223")
	if err != nil {
		log.Printf("Failed to connect to NATS Streaming server: %s", err.Error())
		return nil, nil, nil
	}

	stanCon, err := stan.Connect("test-cluster", "test-user1", stan.NatsConn(natsCon))
	if err != nil {
		log.Printf("Failed to connect to NATS Streaming channel: %s", err.Error())
		return nil, nil, nil
	}

	stanCon.NatsConn().AuthRequired()

	subscribeNats, err := stanCon.Subscribe("test-channel", func(message *stan.Msg) {

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

		var mutex sync.Mutex

		//adding an address to the cache and database
		mutex.Lock()
		cache[newOrder.OrderUID] = newOrder

		connectDB.QueryRow(fmt.Sprintf(`INSERT INTO "orders" ("order_id","order_info") VALUES('%s','%s')`, newOrder.OrderUID, newOrderInfo))
		log.Printf("Added order uid: %s", newOrder.OrderUID)

		mutex.Unlock()

		//Durable subscription so as not to lose data due to connection problems, etc.
	}, stan.DurableName("my-durable"))

	if err != nil {
		log.Printf("Failed to subscribe to NATS Streaming channel: %s", err.Error())
		return nil, nil, nil
	}
	return natsCon, stanCon, subscribeNats
}

// Recovery data to cache from a database
func cacheRecovery(connectDB *sql.DB) {

	orders, err := connectDB.Query(`SELECT "order_id", "order_info" FROM orders`)
	if err != nil {
		log.Println(err.Error())
	}

	var orderCounter int
	for orders.Next() {
		var orderID string
		var orderInfo []byte
		var order order
		err = orders.Scan(&orderID, &orderInfo)
		if err != nil {
			log.Printf("Failed to scan orders: %s", err.Error())
		}
		err := json.Unmarshal(orderInfo, &order)
		if err != nil {
			log.Printf("Failed to parse JSON: %s", err.Error())
		}
		cache[orderID] = order
		orderCounter++
	}
	log.Printf("Cache recovery. Recovery: %v orders", orderCounter)
}

func handler(conf setting) {
	mx := http.NewServeMux()
	srv := &http.Server{
		Addr:    conf.Server,
		Handler: mx,
	}

	mx.HandleFunc("/receiving", getOrder)
	mx.HandleFunc("/health_check", healthCheckHandler)

	log.Printf("Server: %s is running.", conf.Server)
	err := srv.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

// Outputting data to an html form
func getOrder(w http.ResponseWriter, r *http.Request) {
	template, err := template.ParseFiles("web/main.html")
	if err != nil {
		log.Printf("Parse html file error: %s", err.Error())
	}

	uuid := r.FormValue("uuid")

	var orderJson []byte
	_, ok := cache[uuid]

	//Checking the received key for availability in the cache
	switch {
	case uuid == "":
		orderJson = []byte("Введите id")
	case !ok:
		orderJson = []byte("Введен неверный id")
	default:
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
			return
		}
		orderJson = orderJsonFormat.Bytes()
	}

	template, _ = template.ParseFiles("web/main.html")

	err = template.Execute(w, string(orderJson))
	if err != nil {
		log.Printf("Failed to execute template: %v\n", err)
	}
}

// Checking the server operation
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "The server is working correctly")
}
