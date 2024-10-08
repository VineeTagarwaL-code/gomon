package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
)

var (
	currentCmd         *exec.Cmd
	scriptCrashed      bool
	scriptCrashedMutex sync.Mutex
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Please provide a file to watch & runner")
	}
	runner := os.Args[1]
	scriptTorun := os.Args[2]
	watchDir := filepath.Dir(scriptTorun)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()
	done := make(chan bool)
	go watchForChanges(watcher, scriptTorun, runner)
	err = watcher.Add(watchDir)
	if err != nil {
		log.Fatal(err)
	}
	startScript(scriptTorun, runner, false)
	<-done
}

func watchForChanges(watcher *fsnotify.Watcher, scriptToRun string, runner string) {
	var timer *time.Timer
	debounceDelay := 100 * time.Millisecond
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(debounceDelay, func() {
					scriptCrashedMutex.Lock()
					crashed := scriptCrashed
					scriptCrashedMutex.Unlock()

					if crashed {
						color.Yellow("\nScript was crashed. Restarting due to changes...")
						startScript(scriptToRun, runner, true)
					} else {
						color.Green("\nChanges Detected. Restarting script...\n")
						startScript(scriptToRun, runner, true)
					}
				})
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Error:", err)
		}
	}
}

func startScript(scriptTorun string, runner string, restart bool) {
	if currentCmd != nil && currentCmd.Process != nil {
		pgid, err := syscall.Getpgid(currentCmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGINT)
		}
		time.Sleep(time.Second)
	}

	if !restart {
		color.Blue("\n[ GOMON ] Directory Watcher Added %s", scriptTorun)
		color.Blue("[ GOMON ] Restart Added %s", scriptTorun)
		color.Blue("[ GOMON ] Starting script %s\n", scriptTorun)
	}

	currentCmd = exec.Command(runner, scriptTorun)
	currentCmd.Stdout = os.Stdout
	currentCmd.Stderr = os.Stderr
	currentCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err := currentCmd.Start()
	if err != nil {
		log.Fatal(err)
		return
	}

	scriptCrashedMutex.Lock()
	scriptCrashed = false
	scriptCrashedMutex.Unlock()

	go func() {
		err := currentCmd.Wait()
		if err != nil {
			color.Red("\nScript exited with error: %v\n", err)
			color.Yellow("[ GOMON ] Waiting for file changes before restarting...")
			scriptCrashedMutex.Lock()
			scriptCrashed = true
			scriptCrashedMutex.Unlock()
		}
	}()
}
