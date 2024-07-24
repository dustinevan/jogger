package api

import (
	"context"
	"fmt"
	"github.com/dustinevan/jogger/lib/job"
	"go.uber.org/zap"

	jogv1 "github.com/dustinevan/jogger/pkg/gen/jogger/v1"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// Server is the implementation of the grpc JobServiceServer
type Server struct {
	jogv1.UnimplementedJobServiceServer
	manager *job.Manager
	log     *zap.SugaredLogger
}

func NewServer(manager *job.Manager, log *zap.SugaredLogger) *Server {
	return &Server{manager: manager, log: log}
}

// Start starts a new job
func (s Server) Start(ctx context.Context, req *jogv1.StartRequest) (*jogv1.StartResponse, error) {
	s.log.Infow("starting job", "cmd", req.Job.GetCmd(), "args", req.Job.GetArgs())
	username, err := CommonNameFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting job: %w", err)
	}
	//
	jobID, err := s.manager.Start(ctx, username, req.Job.GetCmd(), req.Job.GetArgs()...)
	if err != nil {
		return nil, fmt.Errorf("starting job: %w", err)
	}
	s.log.Infow("job started", "jobID", jobID, "username", username)
	return &jogv1.StartResponse{JobId: jobID}, nil
}

// Stop stops a job
func (s Server) Stop(ctx context.Context, req *jogv1.StopRequest) (*jogv1.StopResponse, error) {
	s.log.Infow("stopping job", "jobID", req.JobId)
	username, err := CommonNameFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("stopping job: %w", err)
	}
	err = s.manager.Stop(ctx, username, req.JobId)
	if err != nil {
		return nil, fmt.Errorf("stopping job: %w", err)
	}
	s.log.Infow("job stopped", "jobID", req.JobId, "username", username)
	return &jogv1.StopResponse{}, nil
}

// Status gets the status of a job
func (s Server) Status(ctx context.Context, req *jogv1.StatusRequest) (*jogv1.StatusResponse, error) {
	s.log.Infow("getting job status", "jobID", req.JobId)
	username, err := CommonNameFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting job status: %w", err)
	}
	status, err := s.manager.Status(ctx, username, req.JobId)
	if err != nil {
		return nil, fmt.Errorf("getting job status: %w", err)
	}
	s.log.Infow("job status", "jobID", req.JobId, "status", status, "username", username)
	return &jogv1.StatusResponse{Status: status}, nil
}

// Output streams the output of a job
func (s Server) Output(req *jogv1.OutputRequest, srv jogv1.JobService_OutputServer) error {
	s.log.Infow("streaming output", "jobID", req.JobId)
	username, err := CommonNameFromContext(srv.Context())
	if err != nil {
		return fmt.Errorf("streaming output: %w", err)
	}
	defer s.log.Infow("streaming output complete", "jobID", req.JobId, "username", username)

	stream, err := s.manager.OutputStream(srv.Context(), username, req.JobId)
	if err != nil {
		return fmt.Errorf("streaming output: %w", err)
	}

	// Instead of ranging over the channel, we loop here tp listen for context cancellation.
	for {
		select {
		case <-srv.Context().Done():
			return nil
		case output, ok := <-stream:
			if !ok {
				// The stream has been closed
				return nil
			}
			if err := srv.Send(&jogv1.OutputResponse{Data: &jogv1.OutputData{Data: output}}); err != nil {
				return fmt.Errorf("sending output chunk: %w", err)
			}
		}
	}
}

// CommonNameFromContext gets the common name from peer certificates in the context -- this is the username
// Note that for local development, this is set in the gencerts binary.
func CommonNameFromContext(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("getting common name from context: failed to get peer")
	}
	if p.AuthInfo == nil {
		return "", fmt.Errorf("getting common name from context: no AuthInfo available")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("getting common name from context: no TLSInfo available")
	}
	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", fmt.Errorf("getting common name from context: there are no peer certificates")
	}
	if len(tlsInfo.State.PeerCertificates) > 1 {
		return "", fmt.Errorf("getting common name from context: there are multiple peer certificates")
	}
	if tlsInfo.State.PeerCertificates[0].Subject.CommonName == "" {
		return "", fmt.Errorf("getting common name from context: peer certificate has no common name")
	}
	return tlsInfo.State.PeerCertificates[0].Subject.CommonName, nil
}
