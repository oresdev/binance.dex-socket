package binance

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	cxt    context.Context
	cancel context.CancelFunc
)

func init() {
	cxt, cancel = context.WithCancel(context.Background())
}

// Connect ...
func Connect(conf Conf) *Client {
	client := &Client{Conf: conf}

	client.dialer = &websocket.Dialer{
		HandshakeTimeout:  30 * time.Second,
		EnableCompression: true,
	}

	if conf.PingPeriod == 0 {
		client.PingPeriod = 15 * time.Second
	}

	if conf.ReadDeadlineTime == 0 {
		client.ReadDeadlineTime = 2 * client.PingPeriod
	}

	client.reconnectLock = new(sync.Mutex)

	client.waiter = &sync.WaitGroup{}

	return client
}

func (client *Client) dial() error {
	conn, resp, err := client.dialer.Dial(client.URL, http.Header(client.ReqHeaders))

	if err != nil {
		if resp != nil {
			dumpData, _ := httputil.DumpResponse(resp, true)
			err := errors.New(string(dumpData))
			log.Println(err)
		}
		log.Printf("websocket-client dial %s fail", client.URL)
		return err
	}

	client.conn = conn

	client.conn.SetReadDeadline(time.Now().Add(client.ReadDeadlineTime))

	dumpData, _ := httputil.DumpResponse(resp, true)

	log.Printf("websocket-client connected to %s. response from server: \n %s", client.URL, string(dumpData))

	if client.OnConnect != nil {
		client.OnConnect()
	}

	if client.OnClose != nil {
		client.conn.SetCloseHandler(client.OnClose)
	}

	client.conn.SetPongHandler(func(appData string) error {
		client.conn.SetReadDeadline(time.Now().Add(client.ReadDeadlineTime))
		return nil
	})

	return nil
}

func (client *Client) initBufferChan() {
	client.open = make(chan bool)
	client.writeBuffer = make(chan []byte, 10)
	client.closeMessageBuffer = make(chan []byte, 10)
}

// Start ...
func (client *Client) Start() {
	// buffer channel reset
	client.initBufferChan()

	// dial
	err := client.dial()

	if err != nil {
		log.Println("websocket-client start error:", err)
		if client.IsAutoReconnect {
			client.reconnect(10)
		}
	}

	// start read goroutine and write goroutine
	for {
		client.waiter.Add(2)
		go client.write()
		go client.read()
		client.waiter.Wait()
		close(client.open)
		if client.IsAutoReconnect {
			client.reconnect(10)
		} else {
			log.Println("websocket-client closed. bye")
			return
		}
	}
}

func (client *Client) write() {
	var err error
	ctxW, cancelW := context.WithCancel(cxt)

	pingTicker := time.NewTicker(client.PingPeriod)
	session := time.NewTicker(client.SessionPeriod)

	defer func() {
		pingTicker.Stop()
		session.Stop()
		cancelW()
		client.waiter.Done()
	}()

	for {
		select {
		case <-ctxW.Done():
			log.Println("websocket-client connect closing, exit writing progress...")
			return
		case d := <-client.writeBuffer:
			err = client.conn.WriteMessage(websocket.TextMessage, d)
		case d := <-client.closeMessageBuffer:
			err = client.conn.WriteMessage(websocket.CloseMessage, d)
		case <-pingTicker.C:
			err = client.conn.WriteMessage(websocket.PingMessage, []byte("ping"))
		case <-session.C:
			err = client.conn.WriteMessage(websocket.TextMessage, []byte(`{"method": "keepAlive"}`))
		default:
			err = nil
		}
		if err != nil {
			log.Printf("write error: %v", err)
			return
		}
	}
}

func (client *Client) read() {
	ctxR, cancelR := context.WithCancel(cxt)

	defer func() {
		client.conn.Close()
		cancelR()
		client.waiter.Done()
	}()

	for {
		select {
		case <-ctxR.Done():
			log.Println("websocket-client connect closing, exit receiving progress...")
			return
		default:
			client.conn.SetReadDeadline(time.Now().Add(client.ReadDeadlineTime))
			_, msg, err := client.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
					log.Printf("read error: %v", err)
				}
				return
			}
			if client.OnMessage != nil {
				client.OnMessage(msg)
			}
		}
	}
}

// Send ...
func (client *Client) Send(msg []byte) {
	client.writeBuffer <- msg
}

// SendClose ...
func (client *Client) SendClose() {
	client.closeMessageBuffer <- websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
}

// Subscribe ...
func (client *Client) Subscribe(data []byte) {
	client.writeBuffer <- data
	client.Subs = append(client.Subs, data)
}

// Terminate ...
func (client *Client) Terminate() {
	client.reconnectLock.Lock()

	client.IsAutoReconnect = false

	defer client.reconnectLock.Unlock()

	cancel()

	// waiting for read_goroutine and write_goroutine are both closed
	<-client.open
}

func (client *Client) reconnect(retry int) {
	client.reconnectLock.Lock()

	defer client.reconnectLock.Unlock()

	var err error

	client.initBufferChan()

	for i := 0; i < retry; i++ {
		err = client.dial()
		if err != nil {
			log.Printf("websocket-client reconnect fail: %d", i)
		} else {
			break
		}
		time.Sleep(client.ReconnectPeriod)
	}

	if err != nil {
		log.Println("websocket-client retry reconnect fail. exiting....")
		client.Terminate()
	} else {
		for _, sub := range client.Subs {
			log.Println("re-subscribe: ", string(sub))
			client.Send(sub)
		}
	}
}

// UnmarshalJSON ...
func UnmarshalJSON(j []byte) Transfer {
	var t Transfer
	if err := json.Unmarshal(j, &t); err != nil {
		log.Println("Error parsing JSON strings - :", err)
	}

	return t
}
