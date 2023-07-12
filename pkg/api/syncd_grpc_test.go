package api

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"net"
	"sync"
	"testing"
)

func TestPublisher(t *testing.T) {

	// Given
	records := []*Record{
		{
			Fields: map[string]*Data{
				"id": &Data{
					Type:  DataType_STRING,
					Value: &Data_String_{String_: uuid.New().String()},
				},
				"attr1": &Data{
					Type:  DataType_FLOAT,
					Value: &Data_Float{Float: 1.2345},
				},
				"attr2": &Data{
					Type:  DataType_BOOL,
					Value: &Data_Bool{Bool: true},
				},
				"version": &Data{
					Type:  DataType_INT,
					Value: &Data_Int{Int: -1},
				},
			},
		},
		{
			Fields: map[string]*Data{
				"id": &Data{
					Type:  DataType_STRING,
					Value: &Data_String_{String_: uuid.New().String()},
				},
				"attr1": &Data{
					Type:  DataType_FLOAT,
					Value: &Data_Float{Float: 2.3456},
				},
				"attr2": &Data{
					Type:  DataType_BOOL,
					Value: &Data_Bool{Bool: false},
				},
				"version": &Data{
					Type:  DataType_INT,
					Value: &Data_Int{Int: 3},
				},
			},
		},
	}
	out := make(chan *Record)
	go func(out chan<- *Record) {
		defer close(out)
		for i := range records {
			out <- records[i]
		}
	}(out)

	publisher := &MockPublisherServer{}
	publisher.TestData().Set("records", out)
	wg := &sync.WaitGroup{}
	wg.Add(2)
	result := &struct {
		Metadata       Metadata
		RecordStatuses []*RecordStatus
		Errors         []error
	}{}
	publisher.On("Pull", mock.Anything).Run(func(args mock.Arguments) {
		// Stream provided test records back to the client; copy all statuses into result
		defer wg.Done()
		server := args.Get(0).(Publisher_PullServer)
		result.Metadata = MustGetMetadataFromContext(server.Context())
		go func() {
			defer wg.Done()
			i, e := server.Recv()
			for ; e == nil; i, e = server.Recv() {
				result.RecordStatuses = append(result.RecordStatuses, i)
			}
			if e != io.EOF {
				t.Errorf("error receiving SyncStatus stream: %v", e)
			}
		}()

		for i := range publisher.TestData().Get("records").Data().(chan *Record) {
			if e := server.Send(i); e != nil {
				result.Errors = append(result.Errors, e)
				break
			}
		}

	}).Return(nil)

	// When

	// .. connection
	ctx := context.Background()
	buf := bufconn.Listen(1024 * 100)
	defer func() { _ = buf.Close() }()

	// .. server
	server := grpc.NewServer()
	RegisterPublisherServer(server, publisher)
	go func() {
		_ = server.Serve(buf)
	}()

	// .. client
	var conn *grpc.ClientConn
	if c, e := grpc.DialContext(ctx, "",
		grpc.WithContextDialer(func(c context.Context, _ string) (net.Conn, error) {
			return buf.DialContext(c)
		}),
		grpc.WithCredentialsBundle(insecure.NewBundle())); e == nil {
		conn = c
	} else {
		t.Fatalf("error creating buffered connection: %v", e)
	}
	client := NewPublisherClient(conn)
	ctx = metadata.AppendToOutgoingContext(context.Background(), "model", "test", "peer", "a-peer")
	if pull, err := client.Pull(ctx); err == nil {
		pull.
		r, e := pull.Recv()
		for ; e == nil; r, e = pull.Recv() {
			_ = pull.Send(&RecordStatus{
				Id:      r.Fields["id"],
				Version: r.Fields["version"],
			})
		}
		if e != io.EOF {
			t.Errorf("error receiving records: %v", e)
		}
		//_ = pull.CloseSend()
	} else {
		t.Errorf("error pulling: %v", err)
	}
	wg.Wait()
	_ = conn.Close()

	// Then
	assert.Equal(t, "a-peer", result.Metadata.Peer)
	assert.Equal(t, "test", result.Metadata.Model)
	assert.Empty(t, result.Errors)
}

type MockPublisherServer struct {
	mock.Mock
	UnimplementedPublisherServer
}

func (m *MockPublisherServer) Pull(server Publisher_PullServer) error {
	return m.MethodCalled("Pull", server).Error(0)
}
