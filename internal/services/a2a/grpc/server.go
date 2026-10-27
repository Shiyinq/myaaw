package grpc

import (
	"context"
	"fmt"
	"strings"

	a2apb "github.com/a2aproject/a2a-go/v2/a2apb/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"

	"myaaw/internal/services/a2a"
)

// Server implements a2apb.A2AServiceServer over a shared a2a.Service.
type Server struct {
	a2apb.UnimplementedA2AServiceServer
	service *a2a.Service
}

// New creates a Server bound to the given service.
func New(service *a2a.Service) *Server {
	return &Server{service: service}
}

// Register attaches the server to a gRPC server.
func (s *Server) Register(server *grpc.Server) {
	a2apb.RegisterA2AServiceServer(server, s)
}

// caller authenticates the request and enforces scope and rate limits.
func (s *Server) caller(ctx context.Context, scope string) (a2a.Caller, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	version := firstValue(md, "a2a-version")
	if version == "" {
		return a2a.Caller{}, versionError(fmt.Errorf("missing a2a-version metadata (supported: %s)", a2a.SupportedProtocolVersion))
	}
	if version != a2a.SupportedProtocolVersion {
		return a2a.Caller{}, versionError(fmt.Errorf("protocol version %s is not supported (supported: %s)", version, a2a.SupportedProtocolVersion))
	}

	caller, ok := s.service.Authenticate(stripBearer(firstValue(md, "authorization")))
	if !ok {
		return a2a.Caller{}, status.Error(codes.Unauthenticated, "unauthorized")
	}
	if !s.service.AllowCaller(caller.Name) {
		return a2a.Caller{}, status.Error(codes.ResourceExhausted, "rate limit exceeded")
	}
	if !caller.Can(scope) {
		return a2a.Caller{}, status.Error(codes.PermissionDenied, "insufficient scope")
	}
	return caller, nil
}

func (s *Server) SendMessage(ctx context.Context, request *a2apb.SendMessageRequest) (*a2apb.SendMessageResponse, error) {
	caller, err := s.caller(ctx, a2a.ScopeDelegate)
	if err != nil {
		return nil, err
	}
	internal := fromProtoSendMessageRequest(request)
	internal.Caller = caller.Name
	task, err := s.service.SendMessage(ctx, internal)
	if err != nil {
		return nil, toStatusError(err)
	}
	return &a2apb.SendMessageResponse{
		Payload: &a2apb.SendMessageResponse_Task{Task: toProtoTask(trimHistory(task, configurationHistoryLength(internal.Configuration)))},
	}, nil
}

func (s *Server) SendStreamingMessage(request *a2apb.SendMessageRequest, stream grpc.ServerStreamingServer[a2apb.StreamResponse]) error {
	caller, err := s.caller(stream.Context(), a2a.ScopeDelegate)
	if err != nil {
		return err
	}
	internal := fromProtoSendMessageRequest(request)
	internal.Caller = caller.Name
	_, err = s.service.SendMessageStream(stream.Context(), internal, func(response a2a.StreamResponse) {
		_ = stream.Send(toProtoStreamResponse(response))
	})
	if err != nil {
		return toStatusError(err)
	}
	return nil
}

func (s *Server) GetTask(ctx context.Context, request *a2apb.GetTaskRequest) (*a2apb.Task, error) {
	caller, err := s.caller(ctx, a2a.ScopeRead)
	if err != nil {
		return nil, err
	}
	task, err := s.service.GetTask(ctx, request.Id)
	if err != nil {
		return nil, toStatusError(err)
	}
	if !s.service.OwnsTask(task, caller.Name) {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	return toProtoTask(trimHistory(task, historyLength(request.HistoryLength))), nil
}

func (s *Server) ListTasks(ctx context.Context, request *a2apb.ListTasksRequest) (*a2apb.ListTasksResponse, error) {
	caller, err := s.caller(ctx, a2a.ScopeRead)
	if err != nil {
		return nil, err
	}
	internal := fromProtoListTasksRequest(request)
	response, err := s.service.ListTasks(ctx, internal, caller.Name)
	if err != nil {
		return nil, toStatusError(err)
	}
	out := &a2apb.ListTasksResponse{
		NextPageToken: response.NextPageToken,
		PageSize:      int32(response.PageSize),
		TotalSize:     int32(response.TotalSize),
		Tasks:         make([]*a2apb.Task, 0, len(response.Tasks)),
	}
	for _, task := range response.Tasks {
		out.Tasks = append(out.Tasks, toProtoTask(trimHistory(task, internal.HistoryLength)))
	}
	return out, nil
}

func (s *Server) CancelTask(ctx context.Context, request *a2apb.CancelTaskRequest) (*a2apb.Task, error) {
	caller, err := s.caller(ctx, a2a.ScopeDelegate)
	if err != nil {
		return nil, err
	}
	if _, err := s.ownedTask(ctx, request.Id, caller); err != nil {
		return nil, err
	}
	task, err := s.service.CancelTask(ctx, request.Id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return toProtoTask(task), nil
}

func (s *Server) SubscribeToTask(request *a2apb.SubscribeToTaskRequest, stream grpc.ServerStreamingServer[a2apb.StreamResponse]) error {
	caller, err := s.caller(stream.Context(), a2a.ScopeDelegate)
	if err != nil {
		return err
	}
	task, err := s.ownedTask(stream.Context(), request.Id, caller)
	if err != nil {
		return err
	}
	if a2a.TerminalStates[task.Status.State] {
		return unsupportedOperationError("task is in a terminal state")
	}
	_, events, active, exists := s.service.SubscribeStream(request.Id)

	send := func(response a2a.StreamResponse) error {
		return stream.Send(toProtoStreamResponse(response))
	}
	if active {
		defer s.service.UnsubscribeStream(request.Id, events)
	}
	if err := send(a2a.StreamResponse{Task: &task}); err != nil {
		return err
	}
	if !active {
		if !exists {
			_ = send(a2a.StreamResponse{StatusUpdate: &a2a.TaskStatusUpdateEvent{
				TaskID:    task.ID,
				ContextID: task.ContextID,
				Status:    task.Status,
			}})
		}
		return nil
	}
	for response := range events {
		if err := send(response); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) CreateTaskPushNotificationConfig(ctx context.Context, request *a2apb.TaskPushNotificationConfig) (*a2apb.TaskPushNotificationConfig, error) {
	caller, err := s.caller(ctx, a2a.ScopeDelegate)
	if err != nil {
		return nil, err
	}
	if _, err := s.ownedTask(ctx, request.TaskId, caller); err != nil {
		return nil, err
	}
	cfg := fromProtoPushConfig(request)
	saved, err := s.service.CreatePushConfig(ctx, request.TaskId, *cfg)
	if err != nil {
		return nil, toStatusError(err)
	}
	return toProtoPushConfig(saved), nil
}

func (s *Server) GetTaskPushNotificationConfig(ctx context.Context, request *a2apb.GetTaskPushNotificationConfigRequest) (*a2apb.TaskPushNotificationConfig, error) {
	caller, err := s.caller(ctx, a2a.ScopeRead)
	if err != nil {
		return nil, err
	}
	if _, err := s.ownedTask(ctx, request.TaskId, caller); err != nil {
		return nil, err
	}
	config, err := s.service.GetPushConfig(ctx, request.TaskId, request.Id)
	if err != nil {
		return nil, toStatusError(err)
	}
	return toProtoPushConfig(config), nil
}

func (s *Server) ListTaskPushNotificationConfigs(ctx context.Context, request *a2apb.ListTaskPushNotificationConfigsRequest) (*a2apb.ListTaskPushNotificationConfigsResponse, error) {
	caller, err := s.caller(ctx, a2a.ScopeRead)
	if err != nil {
		return nil, err
	}
	if _, err := s.ownedTask(ctx, request.TaskId, caller); err != nil {
		return nil, err
	}
	configs, err := s.service.ListPushConfigs(ctx, request.TaskId)
	if err != nil {
		return nil, toStatusError(err)
	}
	out := &a2apb.ListTaskPushNotificationConfigsResponse{Configs: make([]*a2apb.TaskPushNotificationConfig, 0, len(configs))}
	for _, config := range configs {
		out.Configs = append(out.Configs, toProtoPushConfig(config))
	}
	return out, nil
}

func (s *Server) DeleteTaskPushNotificationConfig(ctx context.Context, request *a2apb.DeleteTaskPushNotificationConfigRequest) (*emptypb.Empty, error) {
	caller, err := s.caller(ctx, a2a.ScopeDelegate)
	if err != nil {
		return nil, err
	}
	if _, err := s.ownedTask(ctx, request.TaskId, caller); err != nil {
		return nil, err
	}
	_ = s.service.DeletePushConfig(ctx, request.TaskId, request.Id)
	return &emptypb.Empty{}, nil
}

func (s *Server) GetExtendedAgentCard(ctx context.Context, request *a2apb.GetExtendedAgentCardRequest) (*a2apb.AgentCard, error) {
	if _, err := s.caller(ctx, a2a.ScopeRead); err != nil {
		return nil, err
	}
	if !a2a.ExtendedCardSupported() {
		return nil, unsupportedOperationError("extended agent card is not supported")
	}
	card, err := s.service.ExtendedCard()
	if err != nil {
		return nil, toStatusError(err)
	}
	var result a2apb.AgentCard
	if err := protojson.Unmarshal(card, &result); err != nil {
		return nil, status.Error(codes.Internal, "cannot marshal extended agent card")
	}
	return &result, nil
}

// ownedTask resolves a task and enforces caller ownership.
func (s *Server) ownedTask(ctx context.Context, id string, caller a2a.Caller) (a2a.Task, error) {
	task, err := s.service.GetTask(ctx, id)
	if err != nil {
		return a2a.Task{}, toStatusError(err)
	}
	if !s.service.OwnsTask(task, caller.Name) {
		return a2a.Task{}, status.Error(codes.NotFound, "task not found")
	}
	return task, nil
}

func firstValue(md metadata.MD, key string) string {
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func stripBearer(token string) string {
	token = strings.TrimSpace(token)
	if len(token) > 7 && strings.EqualFold(token[:7], "Bearer ") {
		return strings.TrimSpace(token[7:])
	}
	return token
}

func trimHistory(task a2a.Task, length *int) a2a.Task {
	if length == nil {
		return task
	}
	if *length <= 0 {
		task.Messages = nil
		return task
	}
	if len(task.Messages) > *length {
		task.Messages = task.Messages[len(task.Messages)-*length:]
	}
	return task
}

func historyLength(length *int32) *int {
	if length == nil {
		return nil
	}
	value := int(*length)
	return &value
}

func configurationHistoryLength(config *a2a.SendMessageConfiguration) *int {
	if config == nil {
		return nil
	}
	return config.HistoryLength
}
