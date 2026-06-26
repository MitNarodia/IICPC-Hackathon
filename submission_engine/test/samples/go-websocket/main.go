package main

import (
	"log"
	"net/http"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade failed:", err)
		return
	}
	defer conn.Close()

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		// Echo the message back so bot_fleet can measure the latency round trip
		err = conn.WriteMessage(messageType, message)
		if err != nil {
			break
		}
	}
}

func main() {
	http.HandleFunc("/", handleWebSocket)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	
	log.Println("Mock bot listening on :8081...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
