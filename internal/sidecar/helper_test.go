package sidecar

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestHelperProcess(t *testing.T) {
	index := -1
	for i, arg := range os.Args {
		if arg == "--" {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	if os.Args[index+1] == "crash" {
		os.Exit(3)
	}
	if os.Args[index+1] == "delayed" {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM)
		go func() { <-ch; time.Sleep(150 * time.Millisecond); os.Exit(0) }()
	}
	port := os.Args[len(os.Args)-1]
	fmt.Println("\x1b[32mhelper started\x1b[0m")
	err := http.ListenAndServe("127.0.0.1:"+port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/models") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"models":[{"name":"kev-latest","backend":"mlx","dtype":"bfloat16","device":"mps"}]}`)
	}))
	if err != nil {
		os.Exit(2)
	}
}
