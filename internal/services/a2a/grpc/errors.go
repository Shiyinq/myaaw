package grpc

import (
	"errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"myaaw/internal/services/a2a"
)

// errorInfoDomain is the domain used by the google.rpc.ErrorInfo details.
const errorInfoDomain = "a2a-protocol.org"

// toStatusError maps an A2A service error to a gRPC status with a
// google.rpc.ErrorInfo detail (spec §5.4 and §3.3.2).
func toStatusError(err error) error {
	code, reason := mapError(err)
	if reason == "" {
		return status.Error(code, err.Error())
	}
	detail := &errdetails.ErrorInfo{
		Reason: reason,
		Domain: errorInfoDomain,
	}
	st := status.New(code, err.Error())
	if withDetails, attachErr := st.WithDetails(detail); attachErr == nil {
		return withDetails.Err()
	}
	return st.Err()
}

func mapError(err error) (codes.Code, string) {
	switch {
	case errors.Is(err, a2a.ErrTaskNotFound), errors.Is(err, a2a.ErrPushConfigNotFound):
		return codes.NotFound, "TASK_NOT_FOUND"
	case errors.Is(err, a2a.ErrTaskNotCancelable):
		return codes.FailedPrecondition, "TASK_NOT_CANCELABLE"
	case errors.Is(err, a2a.ErrTaskNotResumable):
		return codes.FailedPrecondition, "UNSUPPORTED_OPERATION"
	case errors.Is(err, a2a.ErrExtendedCardNotConfigured):
		return codes.FailedPrecondition, "EXTENDED_AGENT_CARD_NOT_CONFIGURED"
	case errors.Is(err, a2a.ErrContextMismatch):
		return codes.InvalidArgument, ""
	default:
		return codes.Internal, ""
	}
}

// versionError returns the status for an unsupported protocol version.
func versionError(err error) error {
	detail := &errdetails.ErrorInfo{
		Reason: "VERSION_NOT_SUPPORTED",
		Domain: errorInfoDomain,
	}
	st := status.New(codes.FailedPrecondition, err.Error())
	if withDetails, attachErr := st.WithDetails(detail); attachErr == nil {
		return withDetails.Err()
	}
	return st.Err()
}

// unsupportedOperationError returns the status for a terminal SubscribeToTask.
func unsupportedOperationError(message string) error {
	detail := &errdetails.ErrorInfo{
		Reason: "UNSUPPORTED_OPERATION",
		Domain: errorInfoDomain,
	}
	st := status.New(codes.FailedPrecondition, message)
	if withDetails, attachErr := st.WithDetails(detail); attachErr == nil {
		return withDetails.Err()
	}
	return st.Err()
}
