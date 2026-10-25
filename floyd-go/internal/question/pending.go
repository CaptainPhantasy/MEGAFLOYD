package question

import "slices"

func (s *questionService) Pending() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingRequest == nil {
		return []Request{}
	}
	req := *s.pendingRequest
	req.Questions = slices.Clone(req.Questions)
	for i := range req.Questions {
		req.Questions[i].Choices = slices.Clone(req.Questions[i].Choices)
	}
	return []Request{req}
}
