package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"

	"github.com/petaverse-cloud/pv-global-sync-service/internal/model"
	"github.com/petaverse-cloud/pv-global-sync-service/internal/service"
	"github.com/petaverse-cloud/pv-global-sync-service/pkg/logger"
)

func makeMsg(body string) *primitive.MessageExt {
	return &primitive.MessageExt{
		MsgId:   "msg_001",
		Message: primitive.Message{Body: []byte(body)},
	}
}

// ===== Mocks =====

type mockIdx struct{ insErr, updErr, delErr, statErr error }

func (m *mockIdx) InsertPost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.insErr }
func (m *mockIdx) UpdatePost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.updErr }
func (m *mockIdx) DeletePost(_ context.Context, _ *model.CrossRegionSyncEvent) error { return m.delErr }
func (m *mockIdx) UpdateStats(_ context.Context, _ int64, _, _, _, _ int) error      { return m.statErr }

type mockFeed struct{ newPost, delPost bool }

func (m *mockFeed) HandleNewPost(_ context.Context, _, _ int64) error  { m.newPost = true; return nil }
func (m *mockFeed) HandleDeletedPost(_ context.Context, _ int64) error { m.delPost = true; return nil }

type mockEvtLog struct{ m map[string]bool }

func (m *mockEvtLog) IsProcessed(_ context.Context, id string) (bool, error) { return m.m[id], nil }
func (m *mockEvtLog) MarkProcessed(_ context.Context, ev *model.CrossRegionSyncEvent, _ string) error {
	m.m[ev.EventID] = true
	return nil
}

type mockGDPR struct{ a bool }

func (m *mockGDPR) Check(_ *model.CrossRegionSyncEvent) service.CheckResult {
	return service.CheckResult{Allowed: m.a}
}

type mockAudit struct{}

func (m *mockAudit) Log(_ context.Context, _ *model.CrossRegionSyncEvent, _ bool, _ string) error {
	return nil
}

// ===== routeEvent =====

func TestConsumerRouteEvent_PostCreated(t *testing.T) {
	f := &mockFeed{}
	c := &SyncConsumer{indexSvc: &mockIdx{}, feedGenerator: f, log: logger.NewNop()}
	ev := &model.CrossRegionSyncEvent{EventType: model.EventTypePostCreated, Payload: model.EventPayload{PostUid: 100}}
	if err := c.routeEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if !f.newPost {
		t.Error("HandleNewPost not called")
	}
}

func TestConsumerRouteEvent_PostUpdated(t *testing.T) {
	c := &SyncConsumer{indexSvc: &mockIdx{}, log: logger.NewNop()}
	ev := &model.CrossRegionSyncEvent{EventType: model.EventTypePostUpdated, Payload: model.EventPayload{PostUid: 101}}
	if err := c.routeEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func TestConsumerRouteEvent_PostDeleted(t *testing.T) {
	c := &SyncConsumer{indexSvc: &mockIdx{}, log: logger.NewNop()}
	ev := &model.CrossRegionSyncEvent{EventType: model.EventTypePostDeleted, Payload: model.EventPayload{PostUid: 102}}
	if err := c.routeEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
}

func TestConsumerRouteEvent_Unknown(t *testing.T) {
	c := &SyncConsumer{log: logger.NewNop()}
	if err := c.routeEvent(context.Background(), &model.CrossRegionSyncEvent{EventType: "X"}); err != nil {
		t.Fatal(err)
	}
}

func TestConsumerRouteEvent_Error(t *testing.T) {
	c := &SyncConsumer{indexSvc: &mockIdx{insErr: errors.New("db")}, log: logger.NewNop()}
	ev := &model.CrossRegionSyncEvent{EventType: model.EventTypePostCreated, Payload: model.EventPayload{PostUid: 100}}
	if err := c.routeEvent(context.Background(), ev); err == nil {
		t.Fatal("expected err")
	}
}

// ===== HandleMessage =====

func TestHandleMessage_Success(t *testing.T) {
	el := &mockEvtLog{m: map[string]bool{}}
	c := &SyncConsumer{eventLog: el, gdprChecker: &mockGDPR{true}, auditSvc: &mockAudit{}, indexSvc: &mockIdx{}, feedGenerator: &mockFeed{}, log: logger.NewNop()}
	r, err := c.HandleMessage(context.Background(), makeMsg(`{"eventUid":"e1","eventType":"POST_CREATED","sourceRegion":"SEA","targetRegion":"EU","timestamp":1,"payload":{"postUid":900,"authorUid":800},"metadata":{"gdprCompliant":true,"userConsent":true,"dataCategory":"TIER_2","crossBorderOk":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	if r != consumer.ConsumeSuccess {
		t.Errorf("result=%v", r)
	}
	if !el.m["e1"] {
		t.Error("not marked")
	}
}

func TestHandleMessage_InvalidJSON(t *testing.T) {
	c := &SyncConsumer{log: logger.NewNop()}
	r, err := c.HandleMessage(context.Background(), makeMsg("bad"))
	if err == nil {
		t.Fatal("expected err")
	}
	if r != consumer.ConsumeRetryLater {
		t.Errorf("result=%v", r)
	}
}

func TestHandleMessage_Idempotent(t *testing.T) {
	el := &mockEvtLog{m: map[string]bool{"dup": true}}
	c := &SyncConsumer{eventLog: el, log: logger.NewNop()}
	r, err := c.HandleMessage(context.Background(), makeMsg(`{"eventUid":"dup","eventType":"POST_CREATED","payload":{"postUid":1,"authorUid":2},"metadata":{"dataCategory":"TIER_2"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if r != consumer.ConsumeSuccess {
		t.Error("dup should success")
	}
}

func TestHandleMessage_GDPRDenied(t *testing.T) {
	el := &mockEvtLog{m: map[string]bool{}}
	c := &SyncConsumer{eventLog: el, gdprChecker: &mockGDPR{false}, auditSvc: &mockAudit{}, log: logger.NewNop()}
	r, err := c.HandleMessage(context.Background(), makeMsg(`{"eventUid":"gdpr","eventType":"POST_CREATED","payload":{"postUid":1},"metadata":{"dataCategory":"TIER_1"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if r != consumer.ConsumeSuccess {
		t.Error("deny still success")
	}
	if !el.m["gdpr"] {
		t.Error("marked")
	}
}
