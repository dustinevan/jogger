package command

import (
	"context"
	"errors"
	"fmt"
	jogv1 "github.com/dustinevan/jogger/pkg/gen/jogger/v1"
	"io"
)

func Run(ctx context.Context, client jogv1.JobServiceClient, cmd *Command) error {
	switch cmd.SubCommand {
	case Start:
		return runStart(ctx, client, cmd)
	case Stop:
		return runStop(ctx, client, cmd)
	case Status:
		return runStatus(ctx, client, cmd)
	case Output:
		return runOutput(ctx, client, cmd)
	default:
		return fmt.Errorf("unsupported subcommand: %v", cmd.SubCommand)
	}
}

func runStart(ctx context.Context, client jogv1.JobServiceClient, cmd *Command) error {
	resp, err := client.Start(ctx, &jogv1.StartRequest{Job: &jogv1.Job{Cmd: cmd.RemoteCommand, Args: cmd.RemoteArgs}})
	if err != nil {
		return fmt.Errorf("starting job: %w", err)
	}
	fmt.Printf("job started: %s\n", resp.JobId)
	return nil
}

func runStop(ctx context.Context, client jogv1.JobServiceClient, cmd *Command) error {
	_, err := client.Stop(ctx, &jogv1.StopRequest{JobId: cmd.JobID})
	if err != nil {
		return fmt.Errorf("stopping job: %w", err)
	}
	fmt.Printf("job stopped: %s\n", cmd.JobID)
	return nil
}

func runStatus(ctx context.Context, client jogv1.JobServiceClient, cmd *Command) error {
	resp, err := client.Status(ctx, &jogv1.StatusRequest{JobId: cmd.JobID})
	if err != nil {
		return fmt.Errorf("getting job status: %w", err)
	}
	fmt.Printf("job status: %s\n", resp.Status)
	return nil
}

func runOutput(ctx context.Context, client jogv1.JobServiceClient, cmd *Command) error {
	stream, err := client.Output(ctx, &jogv1.OutputRequest{JobId: cmd.JobID})
	if err != nil {
		return fmt.Errorf("getting job output: %w", err)
	}
	for {
		resp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			err = fmt.Errorf("receiving output: %w", err)
		}
		fmt.Printf("%s", resp.Data.Data)
	}

	closeErr := stream.CloseSend()
	if closeErr != nil {
		if err != nil {
			return fmt.Errorf("%w: error while closing output stream: %s", err, closeErr)
		}
		return fmt.Errorf("closing output stream: %w", closeErr)
	}
	// if there was an error while receiving output, return that error, this will be nil otherwise
	return err
}
