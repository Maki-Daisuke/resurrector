package main

import (
	"fmt"
	"os"
	"os/signal"
)

// wait_for_os_interrupt is a small console test program that blocks until it
// receives os.Interrupt, then reports the signal and waits for Enter before
// exiting.
func main() {
	fmt.Println("Waiting for os.Interrupt...")

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts)
	defer signal.Stop(interrupts)

	sig := <-interrupts
	fmt.Printf("Received signal: %v\n", sig)
	fmt.Print("Press Enter to exit...")

	fmt.Scanln() // Wait for Enter before exiting
}
