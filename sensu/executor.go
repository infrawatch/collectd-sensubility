package sensu

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/infrawatch/apputils/config"
	"github.com/infrawatch/apputils/connector"
	"github.com/infrawatch/apputils/logging"
)

// Standard Sensu return codes
const (
	ExitCodeSuccess = iota
	ExitCodeWarning
	ExitCodeFailure
)

type Executor struct {
	ClientName  string
	TmpBaseDir  string
	ShellPath   string
	log         *logging.Logger
	scriptCache map[string]string
}

func NewExecutor(cfg *config.INIConfig, logger *logging.Logger) (*Executor, error) {
	var executor Executor
	executor.ClientName = cfg.Sections["sensu"].Options["client_name"].GetString()
	executor.TmpBaseDir = cfg.Sections["sensu"].Options["tmp_base_dir"].GetString()
	executor.ShellPath = cfg.Sections["sensu"].Options["shell_path"].GetString()

	executor.scriptCache = make(map[string]string)
	executor.log = logger
	if _, err := os.Stat(executor.TmpBaseDir); os.IsNotExist(err) {
		err := os.MkdirAll(executor.TmpBaseDir, 0700)
		if err != nil {
			return nil, err
		}
	}
	return &executor, nil
}

func (self *Executor) Execute(request connector.CheckRequest) (connector.CheckResult, error) {
	// It is not possible to reasonably exec something like "cmd1 && cmd2 || exit 2".
	// This is usual in Sensu framework so we need to make temporary script for each command.
	// To avoid high IO the script files are cached
	if _, ok := self.scriptCache[request.Command]; !ok {
		scriptFile, err := ioutil.TempFile(self.TmpBaseDir, "check-")
		if err != nil {
			return connector.CheckResult{}, fmt.Errorf("Failed to create temporary file for script: %s", err)
		}
		_, err = scriptFile.Write([]byte(fmt.Sprintf("#!/usr/bin/env sh\n%s\n", request.Command)))
		if err != nil {
			return connector.CheckResult{}, fmt.Errorf("Failed to write script content to temporary file: %s", err)
		}
		self.scriptCache[request.Command] = scriptFile.Name()
		scriptFile.Close()
		self.log.Metadata(map[string]interface{}{"command": request.Command, "path": scriptFile.Name()})
		self.log.Debug("Created check script.")
	}

	cmd := exec.Command(self.ShellPath, self.scriptCache[request.Command])
	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)
	status := ExitCodeSuccess
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				status = ws.ExitStatus()
			} else {
				status = ExitCodeFailure
			}
		} else {
			status = ExitCodeFailure
		}
	}

	outStr := string(output)
	result := connector.CheckResult{
		Client: self.ClientName,
		Result: connector.Result{
			Command:  request.Command,
			Name:     request.Name,
			Issued:   request.Issued,
			Executed: start.Unix(),
			Duration: duration.Seconds(),
			Output:   outStr,
			Status:   status,
		},
	}

	self.log.Metadata(map[string]interface{}{
		"command": request.Command,
		"status":  status,
		"output":  outStr,
	})
	self.log.Debug("Executed check script.")
	return result, nil
}

func (self *Executor) Clean() {
	os.Remove(self.TmpBaseDir)
	self.log.Metadata(map[string]interface{}{"dir": self.TmpBaseDir})
	self.log.Debug("Removed temporary directory.")
}
