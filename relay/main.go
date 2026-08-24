package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/ametow/xpos/relay/xpos"
)

type relayService interface {
	Init() error
	Start()
	Close()
}

func run(service relayService, interrupt <-chan os.Signal) error {
	if err := service.Init(); err != nil {
		return err
	}

	service.Start()
	defer service.Close()
	<-interrupt
	return nil
}

func main() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	defer signal.Stop(ch)

	if err := run(xpos.New(), ch); err != nil {
		log.Fatal(err)
	}
}
