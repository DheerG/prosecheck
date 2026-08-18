package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/DheerG/prosecheck/internal/modelruntime"
)

type installProgressWriter struct {
	writer         io.Writer
	terminal       bool
	lineOpen       bool
	lineWidth      int
	lastBucket     int
	lastLog        time.Time
	completionSent bool
}

func newInstallProgressWriter(writer io.Writer) *installProgressWriter {
	progress := &installProgressWriter{writer: writer, lastBucket: -1}
	file, ok := writer.(*os.File)
	if !ok {
		return progress
	}
	info, err := file.Stat()
	progress.terminal = err == nil && info.Mode()&os.ModeCharDevice != 0
	return progress
}

func (writer *installProgressWriter) Report(progress modelruntime.InstallProgress) {
	if !progress.Transfer {
		writer.Finish()
		fmt.Fprintln(writer.writer, progress.Message)
		writer.resetTransfer()
		return
	}

	line := formatTransferProgress(progress)
	if writer.terminal {
		padding := ""
		if writer.lineWidth > len(line) {
			padding = strings.Repeat(" ", writer.lineWidth-len(line))
		}
		fmt.Fprintf(writer.writer, "\r%s%s", line, padding)
		writer.lineOpen = true
		writer.lineWidth = len(line)
		if progress.Done {
			writer.Finish()
		}
		return
	}

	if !writer.mustLog(progress) {
		return
	}
	fmt.Fprintln(writer.writer, line)
	writer.lastLog = time.Now()
	if progress.Total > 0 {
		writer.lastBucket = progressBucket(progress.Downloaded, progress.Total)
	}
	writer.completionSent = progress.Done
}

func (writer *installProgressWriter) Finish() {
	if writer.lineOpen {
		fmt.Fprintln(writer.writer)
	}
	writer.lineOpen = false
	writer.lineWidth = 0
}

func (writer *installProgressWriter) mustLog(progress modelruntime.InstallProgress) bool {
	if progress.Done && !writer.completionSent {
		return true
	}
	if progress.Total > 0 {
		bucket := progressBucket(progress.Downloaded, progress.Total)
		return writer.lastBucket < 0 || bucket > writer.lastBucket
	}
	return writer.lastLog.IsZero() || time.Since(writer.lastLog) >= 10*time.Second
}

func (writer *installProgressWriter) resetTransfer() {
	writer.lastBucket = -1
	writer.lastLog = time.Time{}
	writer.completionSent = false
}

func progressBucket(downloaded, total int64) int {
	if total <= 0 {
		return 0
	}
	return int(downloaded * 10 / total)
}

func formatTransferProgress(progress modelruntime.InstallProgress) string {
	downloaded := formatByteCount(progress.Downloaded)
	line := fmt.Sprintf("%s: %s", progress.Message, downloaded)
	if progress.Total > 0 {
		percent := float64(progress.Downloaded) * 100 / float64(progress.Total)
		line = fmt.Sprintf("%s / %s (%.0f%%)", line, formatByteCount(progress.Total), percent)
	}
	if progress.BytesPerSecond > 0 {
		line += " at " + formatByteCount(int64(progress.BytesPerSecond)) + "/s"
	}
	return line
}

func formatByteCount(bytes int64) string {
	const (
		kilobyte = int64(1000)
		megabyte = 1000 * kilobyte
		gigabyte = 1000 * megabyte
	)
	switch {
	case bytes >= gigabyte:
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(gigabyte))
	case bytes >= megabyte:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(megabyte))
	case bytes >= kilobyte:
		return fmt.Sprintf("%.1f kB", float64(bytes)/float64(kilobyte))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func modelInstallContext() (context.Context, func()) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
