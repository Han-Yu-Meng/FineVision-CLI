package client

import (
	"bufio"
	"io"
	"os"
)

func StreamResponse(reader io.Reader) error {
	// 使用 bufio.Reader 逐字节或按行读取，不使用 Scanner 避免缓冲区过大的问题
	r := bufio.NewReader(reader)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			os.Stdout.Write(line)
			os.Stdout.Sync()
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
