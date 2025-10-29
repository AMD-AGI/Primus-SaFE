package report

import (
	"context"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	pb "github.com/AMD-AGI/primus-lens/core/pkg/pb/exporter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"time"
)

var (
	grpcConn     *grpc.ClientConn
	client       pb.ExporterServiceClient
	stream       pb.ExporterService_StreamContainerEventsClient
	dockerStream pb.ExporterService_StreamContainerEventsClient
)

func Init(addr string, nodeName, nodeIp string) error {
	var err error
	grpcConn, err = grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(NodeMetadataUnaryInterceptor(nodeName, nodeIp)))
	if err != nil {
		return err
	}
	client = pb.NewExporterServiceClient(grpcConn)
	err = doInitStream(context.Background())
	if err != nil {
		log.Errorf("Failed to initialize stream: %v", err)
		return err
	}
	err = doInitDockerStream(context.Background())
	if err != nil {
		log.Errorf("Failed to initialize docker stream: %v", err)
		return err
	}
	return nil
}

func doInitStream(ctx context.Context) error {
	var err error
	stream, err = client.StreamContainerEvents(ctx)
	if err != nil {
		return err
	}
	go func() {
		for {
			for {
				_, err = stream.Recv()
				if err != nil {
					break
				}
			}
			time.Sleep(1 * time.Second)
			stream, err = client.StreamContainerEvents(ctx)
			if err != nil {
				// Log the error and continue to retry
				log.Errorf("Failed to stream container events: %v", err)
				// This is a simple retry mechanism, you may want to implement a more robust one
				continue
			}
		}
	}()
	return nil
}

func doInitDockerStream(ctx context.Context) error {
	var err error
	dockerStream, err = client.StreamDockerContainerEvents(ctx)
	if err != nil {
		return err
	}
	go func() {
		for {
			for {
				_, err = stream.Recv()
				if err != nil {
					break
				}
			}
			time.Sleep(1 * time.Second)
			dockerStream, err = client.StreamDockerContainerEvents(ctx)
			if err != nil {
				// Log the error and continue to retry
				log.Errorf("Failed to stream docker container events: %v", err)
				// This is a simple retry mechanism, you may want to implement a more robust one
				continue
			}
		}
	}()
	return nil
}

func GetStreamClient() pb.ExporterService_StreamContainerEventsClient {
	return stream
}

func GetDockerStreamClient() pb.ExporterService_StreamDockerContainerEventsClient {
	return dockerStream
}

func NodeMetadataUnaryInterceptor(nodeID, nodeIP string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		md := metadata.Pairs(
			"node_name", nodeID,
			"node_ip", nodeIP,
		)
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}
