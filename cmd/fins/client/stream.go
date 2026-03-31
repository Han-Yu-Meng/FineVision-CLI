package client

import (
	"bufio"
	"io"
	"os"
)

func StreamResponse(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		os.Stdout.Write(line)
		os.Stdout.Write([]byte("\n"))

		os.Stdout.Sync()
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
