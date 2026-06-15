package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"token-router/app/logic/allocator"
	"token-router/conf"
	"token-router/repository/store"
	"token-router/server"
)

func main() {
	if err := conf.Init(); err != nil {
		log.Fatal(err)
	}

	memStore := store.NewMemoryStore(conf.GlobalConfigs.Nodes, conf.GlobalConfigs.Budget)
	alloc := allocator.New(memStore)

	engine := server.NewEngine(alloc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, engine); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
