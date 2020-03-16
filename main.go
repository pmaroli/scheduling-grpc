package main

import (
	"log"

	_ "github.com/lib/pq"
	"github.com/pmaroli/scheduling-rpc/server/rest"
	"github.com/pmaroli/scheduling-rpc/server/rpc"
)

func main() {
	var (
		funcs = []func() error{
			rpc.Start,
			rest.Start,
		}

		errChan = make(chan error)
	)

	for _, f := range funcs {
		go func(f func() error) {
			errChan <- f()
		}(f)
	}

	for err := range errChan {
		log.Fatalf("error: %+v", err)
	}
}
