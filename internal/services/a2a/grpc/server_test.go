package grpc

import (
	"context"
	"database/sql"
	"io"
	"net"
	"path/filepath"
	"testing"

	a2apb "github.com/a2aproject/a2a-go/v2/a2apb/v1"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"myaaw/internal/channel"
	"myaaw/internal/services/a2a"
)

type fakeRunner struct {
	out          *channel.OutgoingMessage
	err          error
	streamChunks []channel.StreamChunk
}

func (f *fakeRunner) BotContext(_ context.Context, _ *channel.IncomingMessage) (*channel.OutgoingMessage, error) {
	return f.out, f.err
}

func (f *fakeRunner) BotStreamContext(_ context.Context, _ *channel.IncomingMessage, onChunk func(channel.StreamChunk)) (*channel.OutgoingMessage, error) {
	for _, chunk := range f.streamChunks {
		onChunk(chunk)
	}
	return f.out, f.err
}

type grpcEnv struct {
	client a2apb.A2AServiceClient
	conn   *grpc.ClientConn
	server *grpc.Server
	done   chan struct{}
}

func newGRPCEnv(t *testing.T, runner a2a.Runner) *grpcEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "a2a.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	service := a2a.NewService(a2a.NewTaskStore(db), runner)
	if err := service.EnsureSchema(); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	New(service).Register(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &grpcEnv{client: a2apb.NewA2AServiceClient(conn), conn: conn, server: server}
}

func v1Context(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "a2a-version", "1.0")
}

func TestGRPCSendAndGetTask(t *testing.T) {
	env := newGRPCEnv(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "hello", ConversationID: "conv-grpc"}})

	sendResp, err := env.client.SendMessage(v1Context(context.Background()), &a2apb.SendMessageRequest{
		Message: &a2apb.Message{Role: a2apb.Role_ROLE_USER, Parts: []*a2apb.Part{{Content: &a2apb.Part_Text{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	task := sendResp.GetTask()
	if task == nil {
		t.Fatalf("response = %+v", sendResp)
	}
	if task.Status.GetState() != a2apb.TaskState_TASK_STATE_COMPLETED {
		t.Fatalf("state = %v", task.Status.GetState())
	}
	if task.ContextId != "conv-grpc" {
		t.Fatalf("contextId = %q", task.ContextId)
	}
	if len(task.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", task.Artifacts)
	}

	getResp, err := env.client.GetTask(v1Context(context.Background()), &a2apb.GetTaskRequest{Id: task.Id})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if getResp.Id != task.Id {
		t.Fatalf("id = %q, want %q", getResp.Id, task.Id)
	}
}

func TestGRPCErrorMapping(t *testing.T) {
	env := newGRPCEnv(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, err := env.client.GetTask(v1Context(context.Background()), &a2apb.GetTaskRequest{Id: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Fatalf("status = %v", st)
	}
	var detail *errdetails.ErrorInfo
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok {
			detail = info
		}
	}
	if detail == nil || detail.GetReason() != "TASK_NOT_FOUND" || detail.GetDomain() != "a2a-protocol.org" {
		t.Fatalf("details = %+v", st.Details())
	}
}

func TestGRPCVersionRequired(t *testing.T) {
	env := newGRPCEnv(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	_, err := env.client.SendMessage(context.Background(), &a2apb.SendMessageRequest{
		Message: &a2apb.Message{Role: a2apb.Role_ROLE_USER, Parts: []*a2apb.Part{{Content: &a2apb.Part_Text{Text: "hi"}}}},
	})
	if err == nil {
		t.Fatal("expected version error")
	}
	if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("code = %v", code)
	}
}

func TestGRPCStreaming(t *testing.T) {
	env := newGRPCEnv(t, &fakeRunner{
		out: &channel.OutgoingMessage{Text: "Hello world"},
		streamChunks: []channel.StreamChunk{
			{Kind: "text", Text: "Hello "},
			{Kind: "text", Text: "world"},
		},
	})

	stream, err := env.client.SendStreamingMessage(v1Context(context.Background()), &a2apb.SendMessageRequest{
		Message: &a2apb.Message{Role: a2apb.Role_ROLE_USER, Parts: []*a2apb.Part{{Content: &a2apb.Part_Text{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("SendStreamingMessage: %v", err)
	}

	var sawTask, sawDelta, sawFinal bool
	for {
		response, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if response.GetTask() != nil {
			sawTask = true
		}
		if response.GetArtifactUpdate() != nil && response.GetArtifactUpdate().Append {
			sawDelta = true
		}
		if response.GetStatusUpdate() != nil {
			sawFinal = true
		}
	}
	if !sawTask || !sawDelta || !sawFinal {
		t.Fatalf("sawTask=%v sawDelta=%v sawFinal=%v", sawTask, sawDelta, sawFinal)
	}
}

func TestGRPCSubscribeTerminalFails(t *testing.T) {
	env := newGRPCEnv(t, &fakeRunner{out: &channel.OutgoingMessage{Text: "ok"}})

	sendResp, err := env.client.SendMessage(v1Context(context.Background()), &a2apb.SendMessageRequest{
		Message: &a2apb.Message{Role: a2apb.Role_ROLE_USER, Parts: []*a2apb.Part{{Content: &a2apb.Part_Text{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	taskID := sendResp.GetTask().Id

	stream, err := env.client.SubscribeToTask(v1Context(context.Background()), &a2apb.SubscribeToTaskRequest{Id: taskID})
	if err != nil {
		t.Fatalf("SubscribeToTask: %v", err)
	}
	if _, err := stream.Recv(); err == nil {
		t.Fatal("expected terminal task subscribe error")
	} else if code := status.Code(err); code != codes.FailedPrecondition {
		t.Fatalf("code = %v, err = %v", code, err)
	}
}
