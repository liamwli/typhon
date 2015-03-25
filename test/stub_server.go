package test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"

	"github.com/b2aio/typhon/server"
)

type StubServer struct {
	server.Server
	t *testing.T

	stubsMutex sync.RWMutex
	stubs      []*ServiceStub
}

type ServiceStub struct {
	ServiceName string
	Endpoint    string
	Handler     func(server.Request) (proto.Message, error)
}

// NewStubServer boots up a regular typhon server and registers a single
// endpoint that subscribes to every routing key
func NewStubServer(t *testing.T) *StubServer {

	s := &StubServer{
		Server: server.NewAMQPServer(),
		t:      t,
	}

	// TODO `Name: "#"` is rather hokey
	s.Init(&server.Config{Name: "#", Description: "Stub Server"})

	go s.Run()

	select {
	case <-s.NotifyConnected():
	case <-time.After(1 * time.Second):
		t.Fatalf("StubServer couldn't connect to RabbitMQ")
	}

	t.Log("[StubServer] Connected to RabbitMQ")

	s.RegisterEndpoint(&server.Endpoint{
		Name: ".*", // TODO Name is not well-named
		Handler: func(req server.Request) (proto.Message, error) {
			return s.handleRequest(req)
		},
	})

	return s
}

// StubResponse is a convenience method to quickly set up stubs that return a fixed value
func (s *StubServer) StubResponse(serviceName, endpoint string, returnValue proto.Message) {
	s.stubResponseAndError(serviceName, endpoint, returnValue, nil)
}

// StubError is a convenience method to stub out a service error
func (s *StubServer) StubError(serviceName, endpoint string, err error) {
	s.stubResponseAndError(serviceName, endpoint, nil, err)
}

// stubResponseAndError registers a stub that returns the passed response and error
func (s *StubServer) stubResponseAndError(serviceName, endpoint string, returnValue proto.Message, err error) {
	s.RegisterStub(&ServiceStub{
		ServiceName: serviceName,
		Endpoint:    endpoint,
		Handler: func(_ server.Request) (proto.Message, error) {
			return returnValue, err
		},
	})
}

// RegisterStub with the server
func (s *StubServer) RegisterStub(stub *ServiceStub) {
	s.stubsMutex.Lock()
	s.t.Logf("[StubServer] Registered stub for %s.%s", stub.ServiceName, stub.Endpoint)
	defer s.stubsMutex.Unlock()
	s.stubs = append(s.stubs, stub)
}

// ResetStubs clears out all server stubs. Test suites should run this between tests
func (s *StubServer) ResetStubs() {
	s.stubsMutex.Lock()
	defer s.stubsMutex.Unlock()
	s.stubs = nil
	s.t.Log("[StubServer] Stubs cleared")
}

// Close cleanly shuts down the stub server
func (s *StubServer) Close() {
	s.Server.Close()
}

// Finds the relevant endpoint stub (if any), and calls its handler function
func (s *StubServer) handleRequest(req server.Request) (proto.Message, error) {

	s.t.Logf("[StubServer] Handling request for %s.%s", req.Service(), req.Endpoint())
	s.stubsMutex.RLock()
	defer s.stubsMutex.RUnlock()

	// determine which endpoint to use
	for _, stub := range s.stubs {
		if stub.ServiceName == req.Service() && stub.Endpoint == req.Endpoint() {
			return stub.Handler(req)
		}
	}
	return nil, fmt.Errorf("No stub found for routing service name %s and endpoint %s", req.Service(), req.Endpoint())
}