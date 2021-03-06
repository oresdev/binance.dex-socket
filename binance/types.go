package binance

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Conf ...
type Conf struct {
	URL              string
	ReqHeaders       map[string][]string
	PingPeriod       time.Duration
	SessionPeriod    time.Duration
	IsAutoReconnect  bool
	OnConnect        func()
	OnMessage        func([]byte)
	OnClose          func(int, string) error
	OnError          func(err error)
	ReadDeadlineTime time.Duration
	ReconnectPeriod  time.Duration
}

// Client ...
type Client struct {
	Conf
	dialer             *websocket.Dialer
	conn               *websocket.Conn
	writeBuffer        chan []byte
	closeMessageBuffer chan []byte
	Subs               [][]byte
	reconnectLock      *sync.Mutex
	waiter             *sync.WaitGroup
	open               chan bool
}

// Subscribe ...
type Subscribe struct {
	Method  string `json:"method"`
	Topic   string `json:"topic"`
	Address string `json:"address"`
}

// Transfer ...
type Transfer struct {
	Stream string      `json:"stream"`
	Data   RawTransfer `json:"data"`
	RawTransfer
}

// RawTransfer ...
type RawTransfer struct {
	TransactionMemo string `json:"M"`
	FromAddr        string `json:"f"`
}
