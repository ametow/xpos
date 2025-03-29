package main

import (
	"log"
	"os"
	"os/signal"
)

func main() {
	xpos := NewXpos()
	err := xpos.Init()
	if err != nil {
		log.Fatal(err)
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	xpos.Start()
	defer xpos.Close()

	<-ch
}
