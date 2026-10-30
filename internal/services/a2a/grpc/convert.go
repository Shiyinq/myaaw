// Package grpc implements the A2A gRPC binding over the shared A2A service.
package grpc

import (
	"encoding/json"
	"time"

	a2apb "github.com/a2aproject/a2a-go/v2/a2apb/v1"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"myaaw/internal/services/a2a"
)

func toProtoState(state string) a2apb.TaskState {
	if value, ok := a2apb.TaskState_value[a2a.WireState(state)]; ok {
		return a2apb.TaskState(value)
	}
	return a2apb.TaskState_TASK_STATE_UNSPECIFIED
}

func fromProtoState(state a2apb.TaskState) string {
	return a2a.StateFromWire(state.String())
}

func toProtoRole(role string) a2apb.Role {
	if value, ok := a2apb.Role_value[a2a.WireRole(role)]; ok {
		return a2apb.Role(value)
	}
	return a2apb.Role_ROLE_UNSPECIFIED
}

func fromProtoRole(role a2apb.Role) string {
	return a2a.RoleFromWire(role.String())
}

func toStruct(value any) *structpb.Struct {
	if value == nil {
		return nil
	}
	mapping, ok := value.(map[string]any)
	if !ok {
		data, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		var converted map[string]any
		if err := json.Unmarshal(data, &converted); err != nil {
			return nil
		}
		mapping = converted
	}
	result, err := structpb.NewStruct(mapping)
	if err != nil {
		return nil
	}
	return result
}

func structToAny(value *structpb.Struct) any {
	if value == nil {
		return nil
	}
	return value.AsMap()
}

func toValue(value any) *structpb.Value {
	if value == nil {
		return nil
	}
	result, err := structpb.NewValue(value)
	if err != nil {
		return nil
	}
	return result
}

func valueToAny(value *structpb.Value) any {
	if value == nil {
		return nil
	}
	return value.AsInterface()
}

func parseTimestamp(value string) *timestamppb.Timestamp {
	if value == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return timestamppb.New(parsed)
		}
	}
	return nil
}

func toProtoPart(part a2a.Part) *a2apb.Part {
	out := &a2apb.Part{
		Filename:  part.Filename,
		MediaType: part.MediaType,
		Metadata:  toStruct(part.Metadata),
	}
	switch {
	case part.Text != "":
		out.Content = &a2apb.Part_Text{Text: part.Text}
	case len(part.Raw) > 0:
		out.Content = &a2apb.Part_Raw{Raw: part.Raw}
	case part.URL != "":
		out.Content = &a2apb.Part_Url{Url: part.URL}
	case part.Data != nil:
		out.Content = &a2apb.Part_Data{Data: toValue(part.Data)}
	}
	return out
}

func fromProtoPart(part *a2apb.Part) a2a.Part {
	if part == nil {
		return a2a.Part{}
	}
	out := a2a.Part{
		Filename:  part.Filename,
		MediaType: part.MediaType,
		Metadata:  structToAny(part.Metadata),
	}
	switch content := part.Content.(type) {
	case *a2apb.Part_Text:
		out.Text = content.Text
	case *a2apb.Part_Raw:
		out.Raw = content.Raw
	case *a2apb.Part_Url:
		out.URL = content.Url
	case *a2apb.Part_Data:
		out.Data = valueToAny(content.Data)
	}
	return out
}

func toProtoMessage(message a2a.Message) *a2apb.Message {
	out := &a2apb.Message{
		MessageId:        message.MessageID,
		ContextId:        message.ContextID,
		TaskId:           message.TaskID,
		Role:             toProtoRole(message.Role),
		Parts:            make([]*a2apb.Part, 0, len(message.Parts)),
		Metadata:         toStruct(message.Metadata),
		Extensions:       message.Extensions,
		ReferenceTaskIds: message.ReferenceTaskIds,
	}
	for _, part := range message.Parts {
		out.Parts = append(out.Parts, toProtoPart(part))
	}
	return out
}

func fromProtoMessage(message *a2apb.Message) a2a.Message {
	if message == nil {
		return a2a.Message{}
	}
	out := a2a.Message{
		MessageID:        message.MessageId,
		ContextID:        message.ContextId,
		TaskID:           message.TaskId,
		Role:             fromProtoRole(message.Role),
		Parts:            make([]a2a.Part, 0, len(message.Parts)),
		Metadata:         structToAny(message.Metadata),
		Extensions:       message.Extensions,
		ReferenceTaskIds: message.ReferenceTaskIds,
	}
	for _, part := range message.Parts {
		out.Parts = append(out.Parts, fromProtoPart(part))
	}
	return out
}

func toProtoArtifact(artifact a2a.Artifact) *a2apb.Artifact {
	out := &a2apb.Artifact{
		ArtifactId:  artifact.ArtifactID,
		Name:        artifact.Name,
		Description: artifact.Description,
		Parts:       make([]*a2apb.Part, 0, len(artifact.Parts)),
		Metadata:    toStruct(artifact.Metadata),
		Extensions:  artifact.Extensions,
	}
	for _, part := range artifact.Parts {
		out.Parts = append(out.Parts, toProtoPart(part))
	}
	return out
}

func toProtoTaskStatus(status a2a.TaskStatus) *a2apb.TaskStatus {
	out := &a2apb.TaskStatus{
		State:     toProtoState(status.State),
		Timestamp: parseTimestamp(status.Timestamp),
	}
	if status.Message != nil {
		out.Message = toProtoMessage(*status.Message)
	}
	return out
}

func toProtoTask(task a2a.Task) *a2apb.Task {
	out := &a2apb.Task{
		Id:        task.ID,
		ContextId: task.ContextID,
		Status:    toProtoTaskStatus(task.Status),
		Artifacts: make([]*a2apb.Artifact, 0, len(task.Artifacts)),
		History:   make([]*a2apb.Message, 0, len(task.Messages)),
		Metadata:  toStruct(task.Metadata),
	}
	for _, artifact := range task.Artifacts {
		out.Artifacts = append(out.Artifacts, toProtoArtifact(artifact))
	}
	for _, message := range task.Messages {
		out.History = append(out.History, toProtoMessage(message))
	}
	return out
}

func toProtoStreamResponse(response a2a.StreamResponse) *a2apb.StreamResponse {
	out := &a2apb.StreamResponse{}
	switch {
	case response.Task != nil:
		out.Payload = &a2apb.StreamResponse_Task{Task: toProtoTask(*response.Task)}
	case response.Message != nil:
		out.Payload = &a2apb.StreamResponse_Message{Message: toProtoMessage(*response.Message)}
	case response.StatusUpdate != nil:
		out.Payload = &a2apb.StreamResponse_StatusUpdate{StatusUpdate: toProtoStatusUpdate(*response.StatusUpdate)}
	case response.ArtifactUpdate != nil:
		out.Payload = &a2apb.StreamResponse_ArtifactUpdate{ArtifactUpdate: toProtoArtifactUpdate(*response.ArtifactUpdate)}
	}
	return out
}

func toProtoStatusUpdate(event a2a.TaskStatusUpdateEvent) *a2apb.TaskStatusUpdateEvent {
	return &a2apb.TaskStatusUpdateEvent{
		TaskId:    event.TaskID,
		ContextId: event.ContextID,
		Status:    toProtoTaskStatus(event.Status),
		Metadata:  toStruct(event.Metadata),
	}
}

func toProtoArtifactUpdate(event a2a.TaskArtifactUpdateEvent) *a2apb.TaskArtifactUpdateEvent {
	return &a2apb.TaskArtifactUpdateEvent{
		TaskId:    event.TaskID,
		ContextId: event.ContextID,
		Artifact:  toProtoArtifact(event.Artifact),
		Append:    event.Append,
		LastChunk: event.LastChunk,
		Metadata:  toStruct(event.Metadata),
	}
}

func fromProtoPushConfig(config *a2apb.TaskPushNotificationConfig) *a2a.PushNotificationConfig {
	if config == nil {
		return nil
	}
	out := &a2a.PushNotificationConfig{
		ID:     config.Id,
		TaskID: config.TaskId,
		URL:    config.Url,
		Token:  config.Token,
	}
	if config.Authentication != nil {
		out.Authentication = &a2a.AuthenticationInfo{
			Scheme:      config.Authentication.Scheme,
			Credentials: config.Authentication.Credentials,
		}
	}
	return out
}

func toProtoPushConfig(config a2a.PushNotificationConfig) *a2apb.TaskPushNotificationConfig {
	out := &a2apb.TaskPushNotificationConfig{
		Id:     config.ID,
		TaskId: config.TaskID,
		Url:    config.URL,
		Token:  config.Token,
	}
	if config.Authentication != nil {
		out.Authentication = &a2apb.AuthenticationInfo{
			Scheme:      config.Authentication.Scheme,
			Credentials: config.Authentication.Credentials,
		}
	}
	return out
}

func fromProtoConfiguration(config *a2apb.SendMessageConfiguration) *a2a.SendMessageConfiguration {
	if config == nil {
		return nil
	}
	out := &a2a.SendMessageConfiguration{
		AcceptedOutputModes: config.AcceptedOutputModes,
		ReturnImmediately:   config.ReturnImmediately,
	}
	if config.HistoryLength != nil {
		length := int(*config.HistoryLength)
		out.HistoryLength = &length
	}
	if config.TaskPushNotificationConfig != nil {
		out.TaskPushNotificationConfig = fromProtoPushConfig(config.TaskPushNotificationConfig)
	}
	return out
}

func fromProtoSendMessageRequest(request *a2apb.SendMessageRequest) a2a.SendMessageRequest {
	out := a2a.SendMessageRequest{
		Tenant:   request.Tenant,
		Metadata: structToAny(request.Metadata),
	}
	if request.Message != nil {
		out.Message = fromProtoMessage(request.Message)
	}
	out.Configuration = fromProtoConfiguration(request.Configuration)
	return out
}

func fromProtoListTasksRequest(request *a2apb.ListTasksRequest) a2a.ListTasksRequest {
	out := a2a.ListTasksRequest{
		Tenant:               request.Tenant,
		ContextID:            request.ContextId,
		Status:               fromProtoState(request.Status),
		PageToken:            request.PageToken,
		StatusTimestampAfter: formatTimestamp(request.StatusTimestampAfter),
	}
	if request.PageSize != nil {
		size := int(*request.PageSize)
		out.PageSize = &size
	}
	if request.HistoryLength != nil {
		length := int(*request.HistoryLength)
		out.HistoryLength = &length
	}
	if request.IncludeArtifacts != nil {
		out.IncludeArtifacts = *request.IncludeArtifacts
	}
	return out
}

func formatTimestamp(value *timestamppb.Timestamp) string {
	if value == nil {
		return ""
	}
	return value.AsTime().UTC().Format(time.RFC3339Nano)
}
