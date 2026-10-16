package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"gapu-chap/internal/domain"
	"gapu-chap/internal/service"
)

type fakeMessageRepo struct {
	messages []domain.Message
}

func (r *fakeMessageRepo) Create(_ context.Context, msg *domain.Message) error {
	msg.ID = int64(len(r.messages) + 1)
	msg.AuthorUsername = "user"
	msg.CreatedAt = time.Now()
	r.messages = append(r.messages, *msg)
	return nil
}

func (r *fakeMessageRepo) ListByRoom(_ context.Context, roomID, beforeID int64, limit int) ([]domain.Message, error) {
	var result []domain.Message
	for _, m := range slices.Backward(r.messages) { // новые первыми
		if m.RoomID != roomID || (beforeID > 0 && m.ID >= beforeID) {
			continue
		}
		result = append(result, m)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (r *fakeMessageRepo) ExistsInRoom(_ context.Context, roomID, messageID int64) (bool, error) {
	for _, m := range r.messages {
		if m.ID == messageID && m.RoomID == roomID {
			return true, nil
		}
	}
	return false, nil
}

// fakeReads хранит last_read_message_id для пар (roomID, userID)
// и, как SQL-запрос, двигает отметку только вперёд.
type fakeReads map[[2]int64]int64

func (f fakeReads) MarkRead(_ context.Context, roomID, userID, messageID int64) (bool, error) {
	key := [2]int64{roomID, userID}
	if f[key] >= messageID {
		return false, nil
	}
	f[key] = messageID
	return true, nil
}

// fakeMembership: множество пар (roomID, userID).
type fakeMembership map[[2]int64]bool

func (f fakeMembership) IsMember(_ context.Context, roomID, userID int64) (bool, error) {
	return f[[2]int64{roomID, userID}], nil
}

const (
	testRoomID int64 = 10
	memberID   int64 = 1
	strangerID int64 = 2
)

type fakePublisher struct {
	published []domain.Message
	reads     []domain.ReadReceipt
}

func (p *fakePublisher) PublishRead(receipt domain.ReadReceipt) {
	p.reads = append(p.reads, receipt)
}

func (p *fakePublisher) PublishMessage(msg *domain.Message) {
	p.published = append(p.published, *msg)
}

func newTestMessageService() (*service.MessageService, *fakeMessageRepo, *fakePublisher) {
	repo := &fakeMessageRepo{}
	members := fakeMembership{{testRoomID, memberID}: true}
	pub := &fakePublisher{}
	return service.NewMessageService(repo, members, fakeReads{}, pub), repo, pub
}

func TestSend_Success(t *testing.T) {
	svc, repo, pub := newTestMessageService()

	msg, err := svc.Send(t.Context(), testRoomID, memberID, "  hello  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Body != "hello" {
		t.Errorf("expected trimmed body, got %q", msg.Body)
	}
	if msg.AuthorID != memberID {
		t.Errorf("expected author %d, got %d", memberID, msg.AuthorID)
	}
	if len(repo.messages) != 1 {
		t.Errorf("expected 1 stored message, got %d", len(repo.messages))
	}
	if len(pub.published) != 1 {
		t.Errorf("message must be published")
	}
}

func TestSend_Validation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"empty", ""},
		{"only spaces", "   \n\t "},
		{"too long", strings.Repeat("я", 4001)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, pub := newTestMessageService()

			_, err := svc.Send(t.Context(), testRoomID, memberID, tt.body)
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("expected ErrValidation, got %v", err)
			}
			if len(repo.messages) != 0 {
				t.Error("invalid message must not be stored")
			}
			if len(pub.published) != 0 {
				t.Error("invalid message must not be published")
			}
		})
	}
}

func TestSend_NotMemberForbidden(t *testing.T) {
	svc, repo, pub := newTestMessageService()

	_, err := svc.Send(t.Context(), testRoomID, strangerID, "hi")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(repo.messages) != 0 {
		t.Error("message from non-member must not be stored")
	}
	if len(pub.published) != 0 {
		t.Error("message from non-member must not be published")
	}
}

func TestHistory_NotMemberForbidden(t *testing.T) {
	svc, _, _ := newTestMessageService()

	_, err := svc.History(t.Context(), testRoomID, strangerID, service.HistoryQuery{})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestHistory_Pagination(t *testing.T) {
	svc, _, _ := newTestMessageService()
	for range 5 {
		if _, err := svc.Send(t.Context(), testRoomID, memberID, "msg"); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	// первая страница: 2 самых новых → id 5, 4
	page1, err := svc.History(t.Context(), testRoomID, memberID, service.HistoryQuery{Limit: 2})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got := ids(page1); !slices.Equal(got, []int64{5, 4}) {
		t.Fatalf("page 1: expected [5 4], got %v", got)
	}

	// следующая страница по курсору → id 3, 2
	page2, err := svc.History(t.Context(), testRoomID, memberID,
		service.HistoryQuery{BeforeID: page1[len(page1)-1].ID, Limit: 2})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if got := ids(page2); !slices.Equal(got, []int64{3, 2}) {
		t.Fatalf("page 2: expected [3 2], got %v", got)
	}
}

func TestHistory_LimitClamped(t *testing.T) {
	svc, _, _ := newTestMessageService()
	for range 150 {
		if _, err := svc.Send(t.Context(), testRoomID, memberID, "msg"); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	tests := []struct {
		name      string
		limit     int
		wantCount int
	}{
		{"default when zero", 0, 50},
		{"clamped to max", 1000, 100},
		{"custom", 7, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.History(t.Context(), testRoomID, memberID, service.HistoryQuery{Limit: tt.limit})
			if err != nil {
				t.Fatalf("history: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Errorf("expected %d messages, got %d", tt.wantCount, len(got))
			}
		})
	}
}

func ids(messages []domain.Message) []int64 {
	result := make([]int64, len(messages))
	for i, m := range messages {
		result[i] = m.ID
	}
	return result
}

func TestMarkRead(t *testing.T) {
	svc, _, pub := newTestMessageService()
	var ids []int64
	for range 3 {
		msg, err := svc.Send(t.Context(), testRoomID, memberID, "msg")
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		ids = append(ids, msg.ID)
	}

	if err := svc.MarkRead(t.Context(), testRoomID, memberID, ids[1]); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	want := domain.ReadReceipt{RoomID: testRoomID, UserID: memberID, MessageID: ids[1]}
	if len(pub.reads) != 1 || pub.reads[0] != want {
		t.Fatalf("expected receipt %+v to be published, got %+v", want, pub.reads)
	}

	// Повтор и шаг назад не меняют отметку и не порождают событий.
	for _, id := range []int64{ids[1], ids[0]} {
		if err := svc.MarkRead(t.Context(), testRoomID, memberID, id); err != nil {
			t.Fatalf("mark read %d: %v", id, err)
		}
	}
	if len(pub.reads) != 1 {
		t.Errorf("read marker must not move backwards, got %d events", len(pub.reads))
	}
}

func TestMarkRead_Errors(t *testing.T) {
	svc, _, pub := newTestMessageService()
	msg, err := svc.Send(t.Context(), testRoomID, memberID, "msg")
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	tests := []struct {
		name      string
		userID    int64
		messageID int64
		wantErr   error
	}{
		{"zero message id", memberID, 0, domain.ErrValidation},
		{"not a member", strangerID, msg.ID, domain.ErrForbidden},
		{"message not in room", memberID, 999, domain.ErrNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.MarkRead(t.Context(), testRoomID, tt.userID, tt.messageID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
	if len(pub.reads) != 0 {
		t.Errorf("failed mark read must not publish, got %+v", pub.reads)
	}
}
