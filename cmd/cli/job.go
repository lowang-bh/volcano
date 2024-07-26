package main

import (
	"github.com/spf13/cobra"

	"volcano.sh/volcano/cmd/cli/util"
	"volcano.sh/volcano/pkg/cli/job"
)

func buildJobCmd() *cobra.Command {
	jobCmd := &cobra.Command{
		Use:   "job",
		Short: "vcctl command line operation job",
	}

	jobCommandMap := map[string]struct {
		Short       string
		RunFunction func(cmd *cobra.Command, args []string)
		InitFlags   func(cmd *cobra.Command)
	}{
		"run": {
			Short: "run job by parameters from the command line",
			RunFunction: func(cmd *cobra.Command, args []string) {
				util.CheckError(cmd, job.RunJob(cmd.Context()))
			},
			InitFlags: job.InitRunFlags,
		},
		"list": {
			Short: "list job information",
			RunFunction: func(cmd *cobra.Command, args []string) {
				util.CheckError(cmd, job.ListJobs(cmd.Context()))
			},
			InitFlags: job.InitListFlags,
		},
		"view": {
			Short: "show job information",
			RunFunction: func(cmd *cobra.Command, args []string) {
				util.CheckError(cmd, job.ViewJob(cmd.Context()))
			},
			InitFlags: job.InitViewFlags,
		},
		"suspend": {
			Short: "abort a job",
			RunFunction: func(cmd *cobra.Command, args []string) {
				util.CheckError(cmd, job.SuspendJob(cmd.Context()))
			},
			InitFlags: job.InitSuspendFlags,
		},
		"resume": {
			Short: "resume a job",
			RunFunction: func(cmd *cobra.Command, args []string) {
				util.CheckError(cmd, job.ResumeJob(cmd.Context()))
			},
			InitFlags: job.InitResumeFlags,
		},
		"delete": {
			Short: "delete a job",
			RunFunction: func(cmd *cobra.Command, args []string) {
				util.CheckError(cmd, job.DeleteJob(cmd.Context()))
			},
			InitFlags: job.InitDeleteFlags,
		},
	}

	for command, config := range jobCommandMap {
		cmd := &cobra.Command{
			Use:   command,
			Short: config.Short,
			Run:   config.RunFunction,
		}
		config.InitFlags(cmd)
		jobCmd.AddCommand(cmd)
	}

	return jobCmd
}
