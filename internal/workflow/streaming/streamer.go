package streaming

import (
    "sync"

    "github.com/google/uuid"
    "github.com/KevinKickass/OpenMachineCore/internal/storage"
)

type EventStreamer struct {
    mu          sync.RWMutex
    subscribers map[uuid.UUID][]chan *storage.ExecutionEvent
}

func NewEventStreamer() *EventStreamer {
    return &EventStreamer{
        subscribers: make(map[uuid.UUID][]chan *storage.ExecutionEvent),
    }
}

func (s *EventStreamer) Subscribe(executionID uuid.UUID) <-chan *storage.ExecutionEvent {
    s.mu.Lock()
    defer s.mu.Unlock()

    ch := make(chan *storage.ExecutionEvent, 100)
    s.subscribers[executionID] = append(s.subscribers[executionID], ch)
    return ch
}

func (s *EventStreamer) Unsubscribe(executionID uuid.UUID, ch <-chan *storage.ExecutionEvent) {
    s.mu.Lock()
    defer s.mu.Unlock()

    subs := s.subscribers[executionID]
    for i, sub := range subs {
        if sub == ch {
            s.subscribers[executionID] = append(subs[:i], subs[i+1:]...)
            close(sub)
            break
        }
    }
}

func (s *EventStreamer) Broadcast(executionID uuid.UUID, event *storage.ExecutionEvent) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    for _, ch := range s.subscribers[executionID] {
        select {
        case ch <- event:
        default:
            // Skip if channel is full
        }
    }
}
