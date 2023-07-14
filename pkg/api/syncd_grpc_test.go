package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"io"
	"net"
	"sync"
	"testing"
)

var strConverter = StringData{}

func TestPublisher(t *testing.T) {

	// Given
	records := []*Record{
		{
			Fields: map[string]*Data{
				"id":      strConverter.From(uuid.NewString()),
				"version": strConverter.From("aaa"),
			},
		},
		{
			Fields: map[string]*Data{
				"id":      strConverter.From(uuid.NewString()),
				"version": strConverter.From("bbb"),
			},
		},
		{
			Fields: map[string]*Data{
				"id":      strConverter.From(uuid.NewString()),
				"version": strConverter.From("ccc"),
			},
		},
	}
	result := &struct {
		Metadata       Metadata
		RecordStatuses []*RecordStatus
		Errors         []error
	}{}

	// When

	// .. Server

	publisher := &MockPublisherServer{}
	publisher.On("Pull", mock.Anything).Run(func(args mock.Arguments) {

		// Copy all statuses into result
		server := args.Get(0).(Publisher_PullServer)
		result.Metadata = MustGetMetadataFromContext(server.Context())
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			i, e := server.Recv()
			for ; e == nil; i, e = server.Recv() {
				// t.Logf("received status %s:%s", i.Id, i.Version)
				result.RecordStatuses = append(result.RecordStatuses, i)
			}
			if e != io.EOF {
				t.Errorf("error receiving SyncStatus stream: %v", e)
			}
			wg.Done()
		}()

		// Stream provided test records back to the client; c
		for i := range records {
			if e := server.Send(records[i]); e != nil {
				result.Errors = append(result.Errors, e)
				break
			}
		}

		wg.Wait()
	}).Return(nil)

	// .. server
	server := grpc.NewServer()
	RegisterPublisherServer(server, publisher)
	var target string
	if tgt, e := Start(server); e == nil {
		target = tgt
	} else {
		t.Errorf("error starting server: %v", e)
	}

	// .. client
	var conn *grpc.ClientConn
	if c, e := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials())); e == nil {
		conn = c
	} else {
		t.Fatalf("error dialing server: %v", e)
	}
	client := NewPublisherClient(conn)
	if pull, err := client.Pull(metadata.AppendToOutgoingContext(context.Background(), "model", "test", "peer", "a-peer")); err == nil {

		r, e := pull.Recv()
		for i := len(records); e == nil; r, e = pull.Recv() {
			i--
			status := &RecordStatus{}
			strConverter.MustDecode(r.Fields["id"], &status.Id)
			strConverter.MustDecode(r.Fields["version"], &status.Version)
			//t.Logf("received record %s:%s", status.Id, status.Version)
			_ = pull.Send(status)
			if i == 0 {
				_ = pull.CloseSend()
			}
		}
		if e != io.EOF {
			t.Errorf("error receiving records: %v", e)
		}
	} else {
		t.Errorf("error pulling: %v", err)
	}
	_ = conn.Close()
	server.Stop()

	// Then
	assert.Equal(t, "a-peer", result.Metadata.Peer)
	assert.Equal(t, "test", result.Metadata.Model)
	assert.Empty(t, result.Errors)
}

func Start(srv *grpc.Server) (string, error) {
	if l, err := net.Listen("tcp", "localhost:0"); err == nil {
		go func() {
			_ = srv.Serve(l)
		}()
		return l.Addr().String(), nil
	} else {
		return "", err
	}
}

type MockPublisherServer struct {
	mock.Mock
	UnimplementedPublisherServer
}

func (m *MockPublisherServer) Pull(server Publisher_PullServer) error {
	return m.MethodCalled("Pull", server).Error(0)
}
