package traffic

import (
	"context"
	"net"
	"testing"
	"time"

	statscommand "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"google.golang.org/grpc"
)

type testStatsService struct {
	statscommand.UnimplementedStatsServiceServer
	request *statscommand.QueryStatsRequest
}

func (s *testStatsService) QueryStats(ctx context.Context, request *statscommand.QueryStatsRequest) (*statscommand.QueryStatsResponse, error) {
	s.request = request
	return &statscommand.QueryStatsResponse{
		Stat: []*statscommand.Stat{
			{Name: "user>>>alice>>>traffic>>>uplink", Value: 100},
			{Name: "user>>>alice>>>traffic>>>downlink", Value: 40},
			{Name: "inbound>>>vmess-main>>>traffic>>>uplink", Value: 25},
			{Name: "ignored>>>value", Value: 10},
		},
	}, nil
}

func TestV2RayStatsClientQueriesGRPCWithoutReset(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen grpc: %v", err)
	}
	server := grpc.NewServer()
	service := &testStatsService{}
	statscommand.RegisterStatsServiceServer(server, service)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	client := NewV2RayStatsClient(2 * time.Second)
	counters, err := client.Query(context.Background(), Target{
		Instance: "edge-us",
		Server:   listener.Addr().String(),
		Scopes:   []string{ScopeUser},
	})
	if err != nil {
		t.Fatalf("query grpc stats: %v", err)
	}
	if service.request == nil || service.request.GetReset_() {
		t.Fatalf("stats query should set reset=false, request=%+v", service.request)
	}
	if len(counters) != 2 {
		t.Fatalf("filtered counters=%d want 2: %+v", len(counters), counters)
	}
	for _, counter := range counters {
		if counter.Scope != ScopeUser || counter.Name != "alice" {
			t.Fatalf("unexpected filtered counter: %+v", counter)
		}
	}
}

func TestParseStatsResponseSupportsFixtureJSON(t *testing.T) {
	counters, err := ParseStatsResponse([]byte(`{"stat":[{"name":"outbound>>>direct>>>traffic>>>uplink","value":"12"}]}`))
	if err != nil {
		t.Fatalf("parse fixture JSON: %v", err)
	}
	if len(counters) != 1 || counters[0].Scope != ScopeOutbound || counters[0].Name != "direct" || counters[0].Value != 12 {
		t.Fatalf("unexpected counters: %+v", counters)
	}
}
