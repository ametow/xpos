package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/ametow/xpos/relay/xpos"
)

func main() {
	xpos := xpos.New()
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
