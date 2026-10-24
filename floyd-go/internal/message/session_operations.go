package message

import "github.com/legacy-ai/floyd/internal/pubsub"

// NotifyDeleted invalidates flushed message caches after an atomic transcript
// operation, then tells every attached client to remove those message rows.
func (s *service) NotifyDeleted(messages []Message) {
	s.mu.Lock()
	for _, msg := range messages {
		if pending, ok := s.pending[msg.ID]; ok {
			if pending.timer != nil {
				pending.timer.Stop()
			}
			delete(s.pending, msg.ID)
		}
	}
	s.mu.Unlock()
	for _, msg := range messages {
		s.Publish(pubsub.DeletedEvent, msg.Clone())
	}
}
