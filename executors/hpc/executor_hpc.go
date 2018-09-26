package hpc

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/kardianos/osext"
	"github.com/sirupsen/logrus"
  "github.com/dgruber/drmaa"

	"gitlab.com/gitlab-org/gitlab-runner/common"
	"gitlab.com/gitlab-org/gitlab-runner/executors"
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
		startMsg = "Using SetUID HPC executor..."
	} else {
		// This was in the original code-base so keeping it for posterity
		if options.User != "" {
			s.Shell().User = options.User
		}

		s.DefaultBuildsDir = os.Expand(s.DefaultBuildsDir, mapping)
		s.DefaultCacheDir = os.Expand(s.DefaultCacheDir, mapping)

		startMsg = "Using HPC executor..."
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

func (s *executor) Run(cmd common.ExecutorCommand) error {
	session, sessionErr := drmaa.MakeSession()
	if sessionErr != nil {
    return fmt.Errorf("Could not create DRMAA session: %v", sessionErr)
	}
	defer session.Exit()

	jobTemplate, jobTemplateErr := session.AllocateJobTemplate()
	if jobTemplateErr != nil {
		return fmt.Errorf("Could not create DRMAA job template: %v", jobTemplateErr)
	}
	defer session.DeleteJobTemplate(&jobTemplate)

	jobTemplate.SetRemoteCommand(s.BuildShell.Command)
	jobTemplate.SetArgs(s.BuildShell.Arguments)

	jobID, errRun := session.RunJob(&jobTemplate)
	if errRun != nil {
		return fmt.Errorf("Could not create DRMAA job process ID: %v", errRun)
	}

	ps, errPS := session.JobPs(jobID)
	if errPS != nil {
		return fmt.Errorf("Error during job status query: %s", errPS)
	}

	for ps != drmaa.PsRunning && errPS == nil {
		fmt.Println("status is: ", ps)
		time.Sleep(time.Millisecond * 500)
		ps, errPS = session.JobPs(jobID)
    if ps.String() == "Done" {
      break
    }
	}

	_, errWait := session.Wait(jobID, drmaa.TimeoutWaitForever)
	if errWait != nil {
		return fmt.Errorf("Error waiting for job %s to finish: %s", jobID, errWait)
	}

	s.Println("Successfully completed HPC job")
	return nil
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

  fmt.Println("About to register the HPC Runner...")
	common.RegisterExecutor("hpc", executors.DefaultExecutorProvider{
		Creator:          creator,
		FeaturesUpdater:  featuresUpdater,
		DefaultShellName: options.Shell.Shell,
	})
}
