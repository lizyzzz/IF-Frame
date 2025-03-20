package main

import (
	"flag"
	"fmt"
	"if-frame/client"
	"if-frame/server"
)

func ClientMain(id, x, y int) {
	cli := client.CreateNewClient(int32(id), int32(x), int32(y), "localhost:8080")
	cli.Run()
}

func ServerMain() {
	svr := server.CreateFrameServer("localhost:8080", 2)
	svr.Run()
}

func main() {
	role := flag.String("role", "", "client or server")
	x := flag.Int("x", 0, "client x")
	y := flag.Int("y", 0, "client y")
	id := flag.Int("id", 0, "client id")
	flag.Parse()

	if *role == "client" {
		ClientMain(*id, *x, *y)
	} else if *role == "server" {
		ServerMain()
	} else {
		fmt.Println("args error!")
	}
}
