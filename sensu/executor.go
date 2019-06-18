package sensu

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/paramite/collectd-sensubility/config"
)

const (
	CHECK_OK     = 0
	CHECK_WARN   = 1
	CHECK_FAILED = 2
)

type CheckResult struct {
	Command  string  `json:"command"`
	Name     string  `json:"name"`
	Issued   int64   `json:"issued"`
	Executed int64   `json:"executed"`
	Duration float64 `json:"duration"`
	Output   string  `json:"output"`
	Status   int     `json:"status"`
}

type Result struct {
	Client string      `json:"client"`
	Check  CheckResult `json:"check"`
}

type Executor struct {
	ClientName  string
	TmpBaseDir  string
	ShellPath   string
	scriptCache map[string]string
}

func NewExecutor(cfg *config.Config) (*Executor, error) {
	var executor Executor
	executor.ClientName = cfg.Sections["sensu"].Options["client_name"].GetString()
	executor.TmpBaseDir = cfg.Sections["sensu"].Options["tmp_base_dir"].GetString()
	executor.ShellPath = cfg.Sections["sensu"].Options["shell_path"].GetString()
	executor.scriptCache = make(map[string]string)
	if _, err := os.Stat(executor.TmpBaseDir); os.IsNotExist(err) {
		err := os.MkdirAll(executor.TmpBaseDir, 0700)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return &executor, nil
}

func (self *Executor) Execute(request CheckRequest) (Result, error) {
	// It is not possible to reasonably exec something like "cmd1 && cmd2 || exit 2".
	// This is usual in Sensu framework so we need to make temporary script for each command.
	// To avoid high IO the script files are cached
	if _, ok := self.scriptCache[request.Command]; !ok {
		scriptFile, err := ioutil.TempFile(self.TmpBaseDir, "check-")
		if err != nil {
			return Result{}, fmt.Errorf("Failed to create temporary file for script: %s", err)
		}
		_, err = scriptFile.Write([]byte(fmt.Sprintf("#!/usr/bin/env sh\n%s\n", request.Command)))
		if err != nil {
			return Result{}, fmt.Errorf("Failed to write script content to temporary file: %s", err)
		}
		self.scriptCache[request.Command] = scriptFile.Name()
		scriptFile.Close()
	}

	//cmdParts := strings.Split(request.Command, " ")
	//command := cmdParts[0]
	//args := []string{}
	//for _, part := range cmdParts[1:] {
	//	if part != "" {
	//		args = append(args, part)
	//	}
	//}
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(self.ShellPath, self.scriptCache[request.Command])
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	cmdOut, cmdErr := stdout.String(), stderr.String()
	status := CHECK_OK
	if err != nil {
		status = CHECK_FAILED
	} else if strings.TrimSpace(cmdErr) != "" {
		status = CHECK_WARN
	}
	result := Result{
		Client: self.ClientName,
		Check: CheckResult{
			Command:  request.Command,
			Name:     request.Name,
			Issued:   request.Issued,
			Executed: start.Unix(),
			Duration: duration.Seconds(),
			Output:   cmdOut + cmdErr,
			Status:   status,
		},
	}

	return result, nil
}

func (self *Executor) Clean() error {
	//TODO: delete tmp dir

	return nil
}
