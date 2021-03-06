package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/oresdev/binance.dex-socket/binance"
)

var (
	connected = make(chan bool, 1)
	block     = make(chan bool, 1)
)

func _onMessage(msg []byte) {
	log.Println("OnMessage: ", string(msg))

	binance.UnmarshalJSON(msg)
}

func _onConnect() {
	log.Println("OnConnect")
	connected <- true
}

func _onClose(code int, text string) error {
	log.Println("OnClose", code, text)
	return nil
}

func main() {
	conf := binance.Conf{
		URL:             "wss://testnet-dex.binance.org/api/ws",
		OnMessage:       _onMessage,
		OnClose:         _onClose,
		OnConnect:       _onConnect,
		PingPeriod:      15 * time.Second,
		ReconnectPeriod: 60 * time.Second,
		SessionPeriod:   1500 * time.Second,
		IsAutoReconnect: true,
	}

	client := binance.Connect(conf)

	go client.Start()

	<-connected

	m, err := json.Marshal(binance.Subscribe{
		Method:  "subscribe",
		Topic:   "transfers",
		Address: "tbnb1qtuf578qs9wfl0wh3vs0r5nszf80gvxd28hkrc",
	})
	if err != nil {
		log.Printf("write error: %v", err)
		return
	}

	client.Subscribe(m)

	<-block
}
