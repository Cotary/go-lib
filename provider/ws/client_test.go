package ws

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// helper to convert http://127.0.0.1 -> ws://127.0.0.1
func httpToWS(rawURL string) string {
	if strings.HasPrefix(rawURL, "https://") {
		return "wss://" + strings.TrimPrefix(rawURL, "https://")
	}
	return "ws://" + strings.TrimPrefix(rawURL, "http://")
}

// echo server: reads and echoes back
func startEchoServer(t *testing.T) (srv *httptest.Server, recvCh chan string) {
	recvCh = make(chan string, 10)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			text := string(msg)
			recvCh <- text
			if err := conn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}
	srv = httptest.NewServer(http.HandlerFunc(handler))
	return
}

func TestSendAndReceive(t *testing.T) {
	srv, recvCh := startEchoServer(t)
	defer srv.Close()

	u := httpToWS(srv.URL)
	client := NewWSClient(u)
	// speed up reconnection in case
	client.reconnectWait = 100 * time.Millisecond

	// capture incoming messages
	recvFromServer := make(chan string, 10)
	client.RegisterOnMessage(func(mt int, data []byte) {
		recvFromServer <- string(data)
	})

	client.Start()
	defer client.Stop()

	// send two messages
	client.Send([]byte("hello1"))
	client.Send([]byte("hello2"))

	// assert server received them
	want := []string{"hello1", "hello2"}
	gotSrv := []string{}
	for len(gotSrv) < len(want) {
		select {
		case m := <-recvCh:
			gotSrv = append(gotSrv, m)
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for server to receive messages, got %v", gotSrv)
		}
	}
	if !reflect.DeepEqual(gotSrv, want) {
		t.Errorf("server got %v, want %v", gotSrv, want)
	}

	// assert client received echoes
	gotCli := []string{}
	for len(gotCli) < len(want) {
		select {
		case m := <-recvFromServer:
			gotCli = append(gotCli, m)
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for client callback, got %v", gotCli)
		}
	}
	if !reflect.DeepEqual(gotCli, want) {
		t.Errorf("client got %v, want %v", gotCli, want)
	}
}

// server that closes on a trigger message, then allows new connections to keep logging
func startFlakyServer(t *testing.T, trigger string) (srv *httptest.Server, recvCh chan string) {
	recvCh = make(chan string, 20)
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			text := string(msg)
			recvCh <- text
			if text == trigger {
				// close normally
				conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
		}
	}
	srv = httptest.NewServer(http.HandlerFunc(handler))
	return
}

func TestReconnectAndContinueSend(t *testing.T) {
	// server will close when it sees "trigger_close"
	srv, recvCh := startFlakyServer(t, "trigger_close")
	defer srv.Close()

	u := httpToWS(srv.URL)
	client := NewWSClient(u)
	// speed up reconnect
	client.reconnectWait = 100 * time.Millisecond

	client.Start()
	defer client.Stop()

	// send sequence: msg1, trigger_close, msg2, msg3
	client.Send([]byte("msg1"))
	client.Send([]byte("trigger_close"))
	client.Send([]byte("msg2"))
	client.Send([]byte("msg3"))

	// collect messages that server actually logs
	want := []string{"msg1", "msg2", "msg3"}
	got := []string{}
	timeout := time.After(3 * time.Second)

	for {
		select {
		case m := <-recvCh:
			// ignore the trigger_close record itself
			if m == "trigger_close" {
				continue
			}
			got = append(got, m)
			if len(got) >= len(want) {
				goto Done
			}
		case <-timeout:
			t.Fatalf("timeout waiting for messages. got %v, want %v", got, want)
		}
	}
Done:
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
