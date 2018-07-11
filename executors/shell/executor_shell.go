package shell

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"fmt"
	"time"

	"github.com/kardianos/osext"
	"github.com/sirupsen/logrus"

	"gitlab.com/gitlab-org/gitlab-runner/common"
	"gitlab.com/gitlab-org/gitlab-runner/executors"
	"gitlab.com/gitlab-org/gitlab-runner/helpers"
)

type executor struct {
	executors.AbstractExecutor
}

func (s *executor) Prepare(options common.ExecutorPrepareOptions) error {
	// expand environment variables to have current directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd: %v", err)
	}

	mapping := func(key string) string {
		switch key {
		case "PWD":
			return wd
		default:
			return ""
		}
	}

	// Check to see if this Shell Runner is a SetUID Runner
	setuid := options.Build.Runner.SetUID

	startMsg := ""

	// If this is a SetUID Runner, we have to explicitly prepare separately
	if setuid {
		setuidValidErr := s.prepareSetUID(options, mapping)
		if setuidValidErr != nil {
			return fmt.Errorf("Could not prepare the SetUID Runner. Failed with error: %v", setuidValidErr)
		}
		startMsg = "Using SetUID Shell executor..."
	} else {
		// This was in the original code-base so keeping it for posterity
		if options.User != "" {
			s.Shell().User = options.User
		}

		s.DefaultBuildsDir = os.Expand(s.DefaultBuildsDir, mapping)
		s.DefaultCacheDir = os.Expand(s.DefaultCacheDir, mapping)

		startMsg = "Using Shell executor..."
	}

	// Pass control to executor
	err = s.AbstractExecutor.Prepare(options)
	if err != nil {
		return err
	}

	s.Println(startMsg)
	return nil
}

func (s *executor) prepareSetUID(options common.ExecutorPrepareOptions, mapping func(string) string) error {
	userValidErr := s.ValidateUser(options)

	if userValidErr != nil {
		return fmt.Errorf("User provided for SetUID Runner is not valid, error code %v", userValidErr)
	}

	// depending on the directory specified in config.toml, appropriately
	// create the directory structure with the correct permissions for the
	// builds and cache dirs.
	s.PrepareSetUIDDirectories(options, mapping)

	// Ensure that we set the login shell to be the validated user from the web UI
	s.Shell().User = s.ValidatedUser

	return nil
}

func (s *executor) killAndWait(cmd *exec.Cmd, waitCh chan error) error {
	for {
		s.Debugln("Aborting command...")
		helpers.KillProcessGroup(cmd)
		select {
		case <-time.After(time.Second):
		case err := <-waitCh:
			return err
		}
	}
}

func (s *executor) Run(cmd common.ExecutorCommand) error {
	c := exec.Command(s.BuildShell.Command, s.BuildShell.Arguments...)
	if c == nil {
		return errors.New("Failed to generate execution command")
	}

	helpers.SetProcessGroup(c)
	defer helpers.KillProcessGroup(c)

	// Fill process environment variables
	c.Env = append(os.Environ(), s.BuildShell.Environment...)

	c.Stdout = s.Trace
	c.Stderr = s.Trace

	if s.BuildShell.PassFile {
		scriptDir, err := ioutil.TempDir("", "build_script")
		if err != nil {
			return err
		}
		defer os.RemoveAll(scriptDir)

		scriptFile := filepath.Join(scriptDir, "script."+s.BuildShell.Extension)
		err = ioutil.WriteFile(scriptFile, []byte(cmd.Script), 0700)
		if err != nil {
			return err
		}

		c.Args = append(c.Args, scriptFile)
	} else {
		c.Stdin = bytes.NewBufferString(cmd.Script)
	}

	// Start a process
	err := c.Start()
	if err != nil {
		return fmt.Errorf("Failed to start process: %s", err)
	}

	// Wait for process to finish
	waitCh := make(chan error)
	go func() {
		err := c.Wait()
		if _, ok := err.(*exec.ExitError); ok {
			err = &common.BuildError{Inner: err}
		}
		waitCh <- err
	}()

	// Support process abort
	select {
	case err = <-waitCh:
		return err

	case <-cmd.Context.Done():
		return s.killAndWait(c, waitCh)
	}
}

func init() {
	// Look for self
	runnerCommand, err := osext.Executable()
	if err != nil {
		logrus.Warningln(err)
	}

	options := executors.ExecutorOptions{
		DefaultSetUID:    false,
		DefaultBuildsDir: "$PWD/builds",
		DefaultCacheDir:  "$PWD/cache",
		SharedBuildsDir:  true,
		Shell: common.ShellScriptInfo{
			Shell:         common.GetDefaultShell(),
			Type:          common.LoginShell,
			RunnerCommand: runnerCommand,
		},
		ShowHostname: false,
	}

	creator := func() common.Executor {
		return &executor{
			AbstractExecutor: executors.AbstractExecutor{
				ExecutorOptions: options,
			},
		}
	}

	featuresUpdater := func(features *common.FeaturesInfo) {
		features.Variables = true
		features.Shared = true

		if runtime.GOOS != "windows" {
			features.Session = true
			features.Terminal = true
		}
	}

	common.RegisterExecutor("shell", executors.DefaultExecutorProvider{
		Creator:          creator,
		FeaturesUpdater:  featuresUpdater,
		DefaultShellName: options.Shell.Shell,
	})
}
