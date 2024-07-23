package api

import (
	"context"
	"fmt"
	"github.com/dustinevan/jogger/lib/job"

	jogv1 "github.com/dustinevan/jogger/pkg/gen/jogger/v1"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// Server is the implementation of the grpc JobServiceServer
type Server struct {
	jogv1.UnimplementedJobServiceServer
	manager *job.Manager
}

func NewServer(manager *job.Manager) *Server {
	return &Server{manager: manager}
}

// Start starts a new job
func (s Server) Start(ctx context.Context, req *jogv1.StartRequest) (*jogv1.StartResponse, error) {

	// Style Note: Personally, I try not to read from the context any "lower" than this layer -- where
	// "lower" means deeper into the API calls. Doing this makes it so the API method signatures
	// give readers a clear picture of the data the API depends on.

	// Get the username from the context and pass it to the manager
	username, err := CommonNameFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting job: %w", err)
	}
	//
	jobID, err := s.manager.Start(ctx, username, req.Job.GetCmd(), req.Job.GetArgs()...)
	if err != nil {
		return nil, fmt.Errorf("starting job: %w", err)
	}
	return &jogv1.StartResponse{JobId: jobID}, nil
}

// Stop stops a job
func (s Server) Stop(ctx context.Context, req *jogv1.StopRequest) (*jogv1.StopResponse, error) {
	username, err := CommonNameFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("stopping job: %w", err)
	}
	err = s.manager.Stop(ctx, username, req.JobId)
	if err != nil {
		return nil, fmt.Errorf("stopping job: %w", err)
	}
	return &jogv1.StopResponse{}, nil
}

// Status gets the status of a job
func (s Server) Status(ctx context.Context, req *jogv1.StatusRequest) (*jogv1.StatusResponse, error) {
	username, err := CommonNameFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting job status: %w", err)
	}
	status, err := s.manager.Status(ctx, username, req.JobId)
	if err != nil {
		return nil, fmt.Errorf("getting job status: %w", err)
	}
	return &jogv1.StatusResponse{Status: status}, nil
}

// Output streams the output of a job
func (s Server) Output(req *jogv1.OutputRequest, srv jogv1.JobService_OutputServer) error {
	username, err := CommonNameFromContext(srv.Context())
	if err != nil {
		return fmt.Errorf("streaming output: %w", err)
	}

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
