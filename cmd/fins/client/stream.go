package client

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fatih/color"
)

func StreamResponse(reader io.Reader) error {
	return StreamResponseWithMessage(reader, "Building...")
}

func StreamResponseWithMessage(reader io.Reader, message string) error {
	startTime := time.Now()
	done := make(chan bool)

	// 使用与 utils.LogSection 类似的颜色格式
	infoColor := color.New(color.FgHiBlue, color.Bold).SprintFunc()
	dimColor := color.New(color.FgHiBlack).SprintFunc()

	// 启动一个定时打印时间的协程
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				// 清除最后一行计时信息
				fmt.Printf("\r\033[K")
				return
			case <-ticker.C:
				elapsed := time.Since(startTime)
				// 模拟 utils.LogSection 的提示风格，但保持在行首刷新
				fmt.Printf("\r\033[K%s %s %s", infoColor("> "), infoColor(message), dimColor(fmt.Sprintf("(%.1fs)", elapsed.Seconds())))
			}
		}
	}()

	// 使用 bufio.Reader 逐字节或按行读取，不使用 Scanner 避免缓冲区过大的问题
	r := bufio.NewReader(reader)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			// 先清除计时行，打印内容，再恢复计时（下一轮 ticker 会打印）
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
