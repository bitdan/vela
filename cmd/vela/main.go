package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"vela/internal/bootstrap"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runInit(os.Args[2:])
		return
	}
	if err := bootstrap.Run(); err != nil {
		log.Fatal(err)
	}
}

func runInit(args []string) {
	flags := flag.NewFlagSet("vela init", flag.ExitOnError)
	branch := flags.String("branch", "main", "initial branch name")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: vela init [-branch name] <directory>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		log.Fatal(err)
	}
	if flags.NArg() != 1 {
		flags.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dir, err := bootstrap.InitializeRepository(ctx, flags.Arg(0), *branch)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Initialized Git repository in %s (branch %s)\n", dir, *branch)
}
