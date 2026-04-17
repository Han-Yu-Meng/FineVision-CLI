package client

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
)

func StreamResponse(reader io.Reader) error {
	return StreamResponseWithMessage(reader, "Building...")
}

func StreamResponseWithMessage(reader io.Reader, message string) error {
	startTime := time.Now()
	done := make(chan bool)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			fmt.Printf("\n%s\n", color.YellowString("Interrupt received, cancelling build..."))
			if closer, ok := reader.(io.Closer); ok {
				closer.Close()
			}
		case <-done:
			return
		}
	}()

	infoColor := color.New(color.FgHiBlue, color.Bold).SprintFunc()
	dimColor := color.New(color.FgHiBlack).SprintFunc()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				fmt.Printf("\r\033[K")
				return
			case <-ticker.C:
				elapsed := time.Since(startTime)
				fmt.Printf("\r\033[K%s %s %s", infoColor("> "), infoColor(message), dimColor(fmt.Sprintf("(%.1fs)", elapsed.Seconds())))
			}
		}
	}()

	r := bufio.NewReader(reader)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			fmt.Printf("\r\033[K")
			os.Stdout.Write(line)
			os.Stdout.Sync()
		}
		if err != nil {
			close(done)
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
