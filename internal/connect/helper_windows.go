//go:build windows

package connect

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

type helperSpec struct {
	Exe  string   `json:"exe"`
	Args []string `json:"args"`
}

func RunHelper(workdir string, parentPID int) int {
	data, err := os.ReadFile(filepath.Join(workdir, "helper.json"))
	if err != nil {
		return 2
	}
	var spec helperSpec
	if err := json.Unmarshal(data, &spec); err != nil || spec.Exe == "" || len(spec.Args) == 0 {
		return 2
	}

	logPath := filepath.Join(workdir, "openvpn.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		_ = os.WriteFile(filepath.Join(workdir, "helper.error"), []byte(fmt.Sprintf("log: %v", err)), 0600)
		return 3
	}
	defer logFile.Close()

	cmd := exec.Command(spec.Exe, spec.Args...)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = os.WriteFile(filepath.Join(workdir, "helper.error"), []byte(fmt.Sprintf("start: %v", err)), 0600)
		return 3
	}

	_ = os.WriteFile(filepath.Join(workdir, "helper.pid"), []byte(fmt.Sprintf("%d", os.Getpid())), 0600)
	_ = os.WriteFile(filepath.Join(workdir, "openvpn.pid"), []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0600)

	exited := make(chan int, 1)
	go func() {
		_ = cmd.Wait()
		exited <- cmd.ProcessState.ExitCode()
	}()

	cancelPath := filepath.Join(workdir, "cancel")
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case code := <-exited:

			return code

		case <-ticker.C:
			if _, err := os.Stat(cancelPath); err == nil {
				_ = cmd.Process.Kill()
				<-exited
				return 0
			}
			if !processAlive(parentPID) {

				_ = cmd.Process.Kill()
				<-exited
				return 1
			}
		}
	}
}

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == 259
}
