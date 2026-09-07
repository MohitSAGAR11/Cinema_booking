package booking

type Service struct {
	store SessionStore
}

func NewService(store SessionStore) *Service {
	return &Service{store}
}

// Book holds a seat and discards the session details.
func (s *Service) Book(b Booking) error {
	_, err := s.store.Hold(b)
	return err
}

func (s *Service) Hold(b Booking) (Booking, error) {
	return s.store.Hold(b)
}

func (s *Service) Confirm(sessionID, userID string) (Booking, error) {
	return s.store.Confirm(sessionID, userID)
}

func (s *Service) Release(sessionID, userID string) error {
	return s.store.Release(sessionID, userID)
}

// Seats returns every seat in a screening that is held or confirmed.
func (s *Service) Seats(movieID string) []Booking {
	return s.store.ListBookings(movieID)
}
