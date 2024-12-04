package main

import (
	"fmt"

	"github.com/xsjk/go-wintun"
)

func main() {

	iface := wintun.Interface{
		Name: "Demo",
		IP:   "10.6.7.7/24",
	}

	if err := iface.Open(); err != nil {
		fmt.Printf("Failed to open interface: %s\n", err)
		return
	}
	defer iface.Close()

	go func() {
		for data := range iface.ReceiveAsync() {
			fmt.Println(wintun.Decode(data))
		}
	}()

	fmt.Scanln()
}
