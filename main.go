package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/hexiaopi/iat-demo/iat"
)

var (
	key    string
	secret string
	appid  string
)

func init() {
	flag.StringVar(&key, "key", "", "iflytek iat key")
	flag.StringVar(&secret, "secret", "", "iflytek iat secret")
	flag.StringVar(&appid, "appid", "", "iflytek iat appid")
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func audio(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Fprintf(w, "upgrade err:%v", err)
		return
	}
	defer conn.Close()

	iflytek := iat.NewIflytek(key, secret, appid)
	iatConn, err := iflytek.Connect()
	if err != nil {
		fmt.Fprintf(w, "iat connect err:%v", err)
		return
	}
	result := make(chan string)
	buffer := make(chan []byte)
	go iflytek.Receive(iatConn, result)
	go iflytek.Send(iatConn, buffer)
	go WriteMessage(conn, result)
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNoStatusReceived) {
				log.Println("read close signal")
				iflytek.Close()
			} else {
				log.Println("read err:", err)
			}
			break
		}
		log.Printf("read message type:%d length:%d", messageType, len(message))
		buffer <- message
	}
}

func WriteMessage(conn *websocket.Conn, data chan string) {
	ticker := time.NewTicker(time.Second * 10)
	for {
		select {
		case message := <-data:
			conn.WriteMessage(websocket.TextMessage, []byte(message))
		case <-ticker.C:
			conn.WriteMessage(websocket.PingMessage, nil)
		}
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "index.html")
}

func main() {
	flag.Parse()
	http.HandleFunc("/", index)
	http.HandleFunc("/audio", audio)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
