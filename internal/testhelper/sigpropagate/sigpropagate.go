package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) == 1 {
		cmd := exec.Command(os.Args[0], "x")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		stdout, _ := cmd.StdoutPipe()
		cmd.Start()

		go func() {
			io.Copy(os.Stdout, stdout)
		}()
	}

	sigcount := 0

	// Wait for interrupt signal to gracefully shutdown the app
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-quit:
			fmt.Printf("[%d] got SIGINT\n", os.Getpid())
			sigcount++

			if sigcount > 5 {
				return
			}
		case <-ticker.C:
			fmt.Printf("[%d] hello\n", os.Getpid())
		}
	}
}
