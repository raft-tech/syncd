// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.12.4
// source: api/syncd.proto

package api

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// ConsumerClient is the client API for Consumer service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ConsumerClient interface {
	Push(ctx context.Context, opts ...grpc.CallOption) (Consumer_PushClient, error)
}

type consumerClient struct {
	cc grpc.ClientConnInterface
}

func NewConsumerClient(cc grpc.ClientConnInterface) ConsumerClient {
	return &consumerClient{cc}
}

func (c *consumerClient) Push(ctx context.Context, opts ...grpc.CallOption) (Consumer_PushClient, error) {
	stream, err := c.cc.NewStream(ctx, &Consumer_ServiceDesc.Streams[0], "/syncd.Consumer/Push", opts...)
	if err != nil {
		return nil, err
	}
	x := &consumerPushClient{stream}
	return x, nil
}

type Consumer_PushClient interface {
	Send(*Record) error
	Recv() (*RecordStatus, error)
	grpc.ClientStream
}

type consumerPushClient struct {
	grpc.ClientStream
}

func (x *consumerPushClient) Send(m *Record) error {
	return x.ClientStream.SendMsg(m)
}

func (x *consumerPushClient) Recv() (*RecordStatus, error) {
	m := new(RecordStatus)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ConsumerServer is the server API for Consumer service.
// All implementations must embed UnimplementedConsumerServer
// for forward compatibility
type ConsumerServer interface {
	Push(Consumer_PushServer) error
	mustEmbedUnimplementedConsumerServer()
}

// UnimplementedConsumerServer must be embedded to have forward compatible implementations.
type UnimplementedConsumerServer struct {
}

func (UnimplementedConsumerServer) Push(Consumer_PushServer) error {
	return status.Errorf(codes.Unimplemented, "method Push not implemented")
}
func (UnimplementedConsumerServer) mustEmbedUnimplementedConsumerServer() {}

// UnsafeConsumerServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ConsumerServer will
// result in compilation errors.
type UnsafeConsumerServer interface {
	mustEmbedUnimplementedConsumerServer()
}

func RegisterConsumerServer(s grpc.ServiceRegistrar, srv ConsumerServer) {
	s.RegisterService(&Consumer_ServiceDesc, srv)
}

func _Consumer_Push_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(ConsumerServer).Push(&consumerPushServer{stream})
}

type Consumer_PushServer interface {
	Send(*RecordStatus) error
	Recv() (*Record, error)
	grpc.ServerStream
}

type consumerPushServer struct {
	grpc.ServerStream
}

func (x *consumerPushServer) Send(m *RecordStatus) error {
	return x.ServerStream.SendMsg(m)
}

func (x *consumerPushServer) Recv() (*Record, error) {
	m := new(Record)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Consumer_ServiceDesc is the grpc.ServiceDesc for Consumer service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Consumer_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "syncd.Consumer",
	HandlerType: (*ConsumerServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Push",
			Handler:       _Consumer_Push_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "api/syncd.proto",
}

// PublisherClient is the client API for Publisher service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type PublisherClient interface {
	Pull(ctx context.Context, opts ...grpc.CallOption) (Publisher_PullClient, error)
}

type publisherClient struct {
	cc grpc.ClientConnInterface
}

func NewPublisherClient(cc grpc.ClientConnInterface) PublisherClient {
	return &publisherClient{cc}
}

func (c *publisherClient) Pull(ctx context.Context, opts ...grpc.CallOption) (Publisher_PullClient, error) {
	stream, err := c.cc.NewStream(ctx, &Publisher_ServiceDesc.Streams[0], "/syncd.Publisher/Pull", opts...)
	if err != nil {
		return nil, err
	}
	x := &publisherPullClient{stream}
	return x, nil
}

type Publisher_PullClient interface {
	Send(*RecordStatus) error
	Recv() (*Record, error)
	grpc.ClientStream
}

type publisherPullClient struct {
	grpc.ClientStream
}

func (x *publisherPullClient) Send(m *RecordStatus) error {
	return x.ClientStream.SendMsg(m)
}

func (x *publisherPullClient) Recv() (*Record, error) {
	m := new(Record)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// PublisherServer is the server API for Publisher service.
// All implementations must embed UnimplementedPublisherServer
// for forward compatibility
type PublisherServer interface {
	Pull(Publisher_PullServer) error
	mustEmbedUnimplementedPublisherServer()
}

// UnimplementedPublisherServer must be embedded to have forward compatible implementations.
type UnimplementedPublisherServer struct {
}

func (UnimplementedPublisherServer) Pull(Publisher_PullServer) error {
	return status.Errorf(codes.Unimplemented, "method Pull not implemented")
}
func (UnimplementedPublisherServer) mustEmbedUnimplementedPublisherServer() {}

// UnsafePublisherServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to PublisherServer will
// result in compilation errors.
type UnsafePublisherServer interface {
	mustEmbedUnimplementedPublisherServer()
}

func RegisterPublisherServer(s grpc.ServiceRegistrar, srv PublisherServer) {
	s.RegisterService(&Publisher_ServiceDesc, srv)
}

func _Publisher_Pull_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(PublisherServer).Pull(&publisherPullServer{stream})
}

type Publisher_PullServer interface {
	Send(*Record) error
	Recv() (*RecordStatus, error)
	grpc.ServerStream
}

type publisherPullServer struct {
	grpc.ServerStream
}

func (x *publisherPullServer) Send(m *Record) error {
	return x.ServerStream.SendMsg(m)
}

func (x *publisherPullServer) Recv() (*RecordStatus, error) {
	m := new(RecordStatus)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Publisher_ServiceDesc is the grpc.ServiceDesc for Publisher service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Publisher_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "syncd.Publisher",
	HandlerType: (*PublisherServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Pull",
			Handler:       _Publisher_Pull_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "api/syncd.proto",
}
