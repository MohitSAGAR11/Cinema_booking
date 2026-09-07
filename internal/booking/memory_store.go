package booking

type MemoryStore struct {
	// seat --> booking
	bookings map[string]Booking // map[SeatId] ==> "A2" --> booking 01
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bookings: map[string]Booking{},
	}
}

func (s *MemoryStore) Book(b Booking) error {
	// seat taken -- error if not do the booking
	if _, exists := s.bookings[b.SeatID]; exists {
		return ErrSeatAlreadybooked;
	}

	s.bookings[b.SeatID] = b
	return nil
}

func (s *MemoryStore) ListBookings(movieID string) []Booking {
	var result []Booking
	for _, b := range s.bookings {
		if b.MovieID == movieID {
			result = append(result, b)
		}
	}
	return result
}